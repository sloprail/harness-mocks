package replay

// Unbuildable says what of a recording an adapter cannot reproduce yet.
type Unbuildable struct{ Reason string }

func (u *Unbuildable) Error() string { return u.Reason }

// MockFailure is a mock that did not run to the end.
type MockFailure struct{ Detail string }

func (m *MockFailure) Error() string { return "the mock failed: " + m.Detail }
