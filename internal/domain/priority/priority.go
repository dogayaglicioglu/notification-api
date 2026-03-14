package priority

const (
	PriorityLow                       = 1
	PriorityMedium                    = 2
	PriorityHigh                      = 3
	PriorityLowLabel    PriorityLabel = "low"
	PriorityMediumLabel PriorityLabel = "medium"
	PriorityHighLabel   PriorityLabel = "high"
)

type PriorityLabel string

func (p PriorityLabel) IsValid() bool {
	switch p {
	case PriorityLowLabel, PriorityMediumLabel, PriorityHighLabel:
		return true
	default:
		return false
	}
}

func PriorityStringToInt(priority PriorityLabel) int {
	switch priority {
	case PriorityLowLabel:
		return PriorityLow
	case PriorityMediumLabel:
		return PriorityMedium
	case PriorityHighLabel:
		return PriorityHigh
	default:
		return PriorityMedium
	}
}

func PriorityIntToString(priority int) string {
	switch priority {
	case PriorityLow:
		return string(PriorityLowLabel)
	case PriorityMedium:
		return string(PriorityMediumLabel)
	case PriorityHigh:
		return string(PriorityHighLabel)
	default:
		return string(PriorityMediumLabel)
	}
}
