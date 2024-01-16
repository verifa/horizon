package hz

type LabelSelector struct {
	// MatchLabels is a map of key/value pairs, used to explicitly match labels
	// of an object.
	MatchLabels map[string]string `json:"matchLabels,omitempty" cue:",opt"`

	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty" cue:",opt"`
}

func (s LabelSelector) Matches(labels map[string]string) bool {
	// Handle any explicit match labels first.
	for k, v := range s.MatchLabels {
		if labels[k] != v {
			return false
		}
	}

	// Handle any match expressions.
	for _, expr := range s.MatchExpressions {
		switch expr.Operator {
		case LabelSelectorOpIn:
			if _, ok := labels[expr.Key]; !ok {
				return false
			}
			found := false
			for _, v := range expr.Values {
				if labels[expr.Key] == v {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case LabelSelectorOpNotIn:
			if _, ok := labels[expr.Key]; !ok {
				return false
			}
			found := false
			for _, v := range expr.Values {
				if labels[expr.Key] == v {
					found = true
					break
				}
			}
			if found {
				return false
			}
		case LabelSelectorOpExists:
			if _, ok := labels[expr.Key]; !ok {
				return false
			}
		case LabelSelectorOpDoesNotExist:
			if _, ok := labels[expr.Key]; ok {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type LabelSelectorRequirement struct {
	Key      string                `json:"key" cue:""`
	Operator LabelSelectorOperator `json:"operator" cue:"\"In\" | \"NotIn\" | \"Exists\" | \"DoesNotExist\""`
	// Values must be non-nil for In or NotIn operators.
	// Otherwise the Values has no effect.
	Values []string `json:"values,omitempty" cue:",opt"`
}

type LabelSelectorOperator string

const (
	LabelSelectorOpIn           LabelSelectorOperator = "In"
	LabelSelectorOpNotIn        LabelSelectorOperator = "NotIn"
	LabelSelectorOpExists       LabelSelectorOperator = "Exists"
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"
)
