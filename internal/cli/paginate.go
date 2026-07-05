package cli

// Paginate slices rows starting at offset for at most limit rows. An offset
// past the end yields an empty (non-nil-length-0) slice rather than an error.
// limit <= 0 means unlimited: every row from offset onward is returned.
func Paginate[T any](rows []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(rows) {
		return []T{}
	}
	rows = rows[offset:]
	if limit <= 0 || limit >= len(rows) {
		return rows
	}
	return rows[:limit]
}
