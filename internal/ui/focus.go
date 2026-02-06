package ui

// FocusManager tracks and rotates focus across panels.
type FocusManager struct {
	Current  string   // ID of the currently focused panel
	Order    []string // Tab order for focus rotation
	OnChange func(from, to string)
}

// Next advances focus to the next panel in order.
// Returns the new current focus ID.
func (f *FocusManager) Next() string {
	if len(f.Order) == 0 {
		return ""
	}
	idx := -1
	for i, id := range f.Order {
		if id == f.Current {
			idx = i
			break
		}
	}
	from := f.Current
	nextIdx := (idx + 1) % len(f.Order)
	f.Current = f.Order[nextIdx]
	if f.OnChange != nil && from != f.Current {
		f.OnChange(from, f.Current)
	}
	return f.Current
}

// Prev advances focus to the previous panel in order.
func (f *FocusManager) Prev() string {
	if len(f.Order) == 0 {
		return ""
	}
	idx := -1
	for i, id := range f.Order {
		if id == f.Current {
			idx = i
			break
		}
	}
	from := f.Current
	nextIdx := idx - 1
	if nextIdx < 0 {
		nextIdx = len(f.Order) - 1
	}
	f.Current = f.Order[nextIdx]
	if f.OnChange != nil && from != f.Current {
		f.OnChange(from, f.Current)
	}
	return f.Current
}

// SetFocus sets focus to the given panel ID.
// Returns true if the ID exists in order.
func (f *FocusManager) SetFocus(id string) bool {
	for _, o := range f.Order {
		if o == id {
			from := f.Current
			f.Current = id
			if f.OnChange != nil && from != id {
				f.OnChange(from, id)
			}
			return true
		}
	}
	return false
}
