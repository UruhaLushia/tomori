package config

type ProcessPriority string

const (
	PriorityLow         ProcessPriority = "low"
	PriorityBelowNormal ProcessPriority = "belowNormal"
	PriorityNormal      ProcessPriority = "normal"
	PriorityAboveNormal ProcessPriority = "aboveNormal"
	PriorityHigh        ProcessPriority = "high"
	PriorityHighest     ProcessPriority = "highest"
)

func (p ProcessPriority) Valid() bool {
	switch p {
	case PriorityLow, PriorityBelowNormal, PriorityNormal, PriorityAboveNormal, PriorityHigh, PriorityHighest:
		return true
	default:
		return false
	}
}
