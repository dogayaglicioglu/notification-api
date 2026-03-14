package status

import "fmt"

type Status int

const (
	StatusPending    Status = 100
	StatusProcessing Status = 150
	StatusSuccess    Status = 200
	StatusFailure    Status = 300
	StatusCancelled  Status = 400
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusProcessing, StatusSuccess, StatusFailure, StatusCancelled:
		return true
	default:
		return false
	}
}

func StatusFromString(s string) (int, bool) {
	var parsed Status
	switch s {
	case "pending":
		parsed = StatusPending
	case "processing":
		parsed = StatusProcessing
	case "success":
		parsed = StatusSuccess
	case "failure":
		parsed = StatusFailure
	case "cancelled":
		parsed = StatusCancelled
	default:
		return -1, false
	}
	if !parsed.IsValid() {
		return -1, false
	}
	return int(parsed), true
}

func StatusIntToString(v int) string {
	return Status(v).String()
}

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusProcessing:
		return "processing"
	case StatusSuccess:
		return "success"
	case StatusFailure:
		return "failure"
	case StatusCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("unknown status %d", s)
	}
}
