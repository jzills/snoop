package ui

// The picker and the live board sort by any column: o moves to the next
// column and O reverses the order. Each column has its natural order
// (newest, largest or most urgent first; names A to Z), which O flips.

type listSort struct {
	col int  // index into the list's columns; 0 is the default
	rev bool // the column's natural order reversed
}

// next moves to the next of n columns, in its natural order.
func (s *listSort) next(n int) {
	s.col, s.rev = (s.col+1)%n, false
}

func (s *listSort) isDefault() bool { return s.col == 0 && !s.rev }

// head marks the sorted column's title with the order's direction: ▾ for
// the natural order, ▴ reversed.
func (s listSort) head(title string, col int) string {
	if col != s.col {
		return title
	}
	if s.rev {
		return title + " ▴"
	}
	return title + " ▾"
}

// less orders by cmp, the column's natural order, honoring rev. Ties keep
// the order they came in, as the callers sort stably.
func (s listSort) less(cmp int) bool {
	if s.rev {
		return cmp > 0
	}
	return cmp < 0
}

// cmpDesc and cmpAsc compare for a natural order of largest first and
// smallest first.
func cmpDesc[T int | int64 | float64](a, b T) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}

func cmpAsc[T int | int64 | float64 | string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
