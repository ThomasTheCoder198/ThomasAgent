package model

// SyncResult counts what one catalog sync did; Skipped includes manual-ref collisions and entries over the cap.
type SyncResult struct {
	Added   int
	Updated int
	Skipped int
}
