package paste

import "time"

// SetClock overrides the service clock in tests.
func SetClock(s *Service, now func() time.Time) { s.now = now }
