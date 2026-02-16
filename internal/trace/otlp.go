package trace

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// OTLPExporter exports traces to an OTLP endpoint
type OTLPExporter struct {
	provider *sdktrace.TracerProvider
	tracer   oteltrace.Tracer
	enabled  bool
}

// NewOTLPExporter creates an OTLP exporter if OTEL_EXPORTER_OTLP_ENDPOINT is set
// Returns nil if endpoint not configured (disabled)
func NewOTLPExporter(ctx context.Context) (*OTLPExporter, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return nil, nil // Disabled
	}

	// Parse endpoint URL to extract host:port and path
	var endpointHost string
	var urlPath string
	var useInsecure bool

	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		// Full URL provided
		parsedURL, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid OTEL_EXPORTER_OTLP_ENDPOINT URL: %w", err)
		}
		endpointHost = parsedURL.Host
		urlPath = parsedURL.Path
		if parsedURL.Scheme == "http" {
			useInsecure = true
		}
	} else {
		// Just host:port provided
		endpointHost = endpoint
		urlPath = "" // Use default path
		useInsecure = true // Assume insecure for local dev
	}

	// Build exporter options
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpointHost),
	}
	if useInsecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if urlPath != "" && urlPath != "/v1/traces" {
		// Only set custom path if it's different from default
		opts = append(opts, otlptracehttp.WithURLPath(urlPath))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "devdeploy"
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
	)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	return &OTLPExporter{
		provider: provider,
		tracer:   provider.Tracer("devdeploy/ralph"),
		enabled:  true,
	}, nil
}

// ExportTrace exports a completed Trace to OTLP
func (e *OTLPExporter) ExportTrace(ctx context.Context, t *Trace) error {
	if e == nil || !e.enabled {
		return nil
	}

	if t.RootSpan == nil {
		return nil // Nothing to export
	}

	// Convert hex string trace ID to set up trace context
	traceID, err := hexToTraceID(t.ID)
	if err != nil {
		return fmt.Errorf("invalid trace ID: %w", err)
	}

	// Create a context with the trace ID
	traceCtx := oteltrace.ContextWithSpanContext(ctx, oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		TraceFlags: oteltrace.FlagsSampled,
	}))

	// Export spans recursively
	e.exportSpan(traceCtx, t.RootSpan, oteltrace.SpanContext{})

	// Force flush the batcher to ensure spans are sent immediately
	if err := e.provider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("failed to flush traces: %w", err)
	}

	return nil
}

// exportSpan recursively exports a span and its children
func (e *OTLPExporter) exportSpan(ctx context.Context, span *Span, parent oteltrace.SpanContext) {
	// Convert hex string IDs to trace IDs
	traceID, err := hexToTraceID(span.TraceID)
	if err != nil {
		return // Skip invalid trace ID
	}

	// Set up context with parent span context (which contains the trace ID)
	// The SDK will extract the trace ID from the parent and create a new span ID
	parentCtx := ctx
	if parent.IsValid() {
		// Parent is valid - use it as the parent context
		// The parent's span context already contains the trace ID
		parentCtx = oteltrace.ContextWithSpanContext(ctx, parent)
	} else {
		// No parent - this is the root span, set trace ID in context
		spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
			TraceID:    traceID,
			TraceFlags: oteltrace.FlagsSampled,
		})
		parentCtx = oteltrace.ContextWithSpanContext(ctx, spanCtx)
	}

	// Create OTLP span with explicit start/end times
	// Note: The SDK will create a new span ID, but we preserve the trace ID
	// and parent relationships. The span structure and timing are preserved.
	_, otlpSpan := e.tracer.Start(
		parentCtx,
		span.Name,
		oteltrace.WithTimestamp(span.StartTime),
	)

	// Map attributes
	attrs := make([]attribute.KeyValue, 0, len(span.Attributes))
	for k, v := range span.Attributes {
		// Map known attributes to devdeploy.* namespace
		var key string
		switch k {
		case "bead_id":
			key = "devdeploy.bead.id"
		case "bead_title":
			key = "devdeploy.bead.title"
		case "tool_name":
			key = "devdeploy.tool.name"
		case "file_path":
			key = "devdeploy.file.path"
		case "command":
			key = "devdeploy.shell.command"
		case "outcome":
			key = "devdeploy.outcome"
		default:
			// Keep other attributes with devdeploy.* prefix
			key = "devdeploy." + k
		}
		attrs = append(attrs, attribute.String(key, v))
	}
	otlpSpan.SetAttributes(attrs...)

	// End the span with explicit end time
	otlpSpan.End(oteltrace.WithTimestamp(span.StartTime.Add(span.Duration)))

	// Get the span context for this span (for children)
	currentSpanCtx := otlpSpan.SpanContext()

	// Recurse for children, passing this span's context as parent
	for _, child := range span.Children {
		e.exportSpan(ctx, child, currentSpanCtx)
	}
}

// hexToTraceID converts a 32-character hex string to trace.TraceID
func hexToTraceID(hexStr string) (oteltrace.TraceID, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return oteltrace.TraceID{}, err
	}
	if len(bytes) != 16 {
		return oteltrace.TraceID{}, err
	}
	var traceID oteltrace.TraceID
	copy(traceID[:], bytes)
	return traceID, nil
}

// Shutdown flushes and closes the exporter
func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	if e == nil {
		return nil
	}
	return e.provider.Shutdown(ctx)
}
