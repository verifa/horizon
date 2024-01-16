package hz

import "testing"

func TestLabelSelector(t *testing.T) {
	t.Parallel()

	type testcase struct {
		name     string
		labels   map[string]string
		selector LabelSelector
		match    bool
	}
	tcs := []testcase{
		{
			name: "match_true",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			match: true,
		},
		{
			name: "match_false",
			labels: map[string]string{
				"foo": "bar",
				"zoo": "bar",
			},
			selector: LabelSelector{
				MatchLabels: map[string]string{
					"foo": "zoo",
				},
			},
			match: false,
		},
		{
			name: "expr_in_true",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: LabelSelectorOpIn,
						Values:   []string{"bar"},
					},
				},
			},
			match: true,
		},
		{
			name: "expr_in_false",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: LabelSelectorOpIn,
						Values:   []string{"zoo"},
					},
				},
			},
			match: false,
		},
		{
			name: "expr_not_in_true",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: LabelSelectorOpNotIn,
						Values:   []string{"zoo"},
					},
				},
			},
			match: true,
		},
		{
			name: "expr_not_in_false",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: LabelSelectorOpNotIn,
						Values:   []string{"bar"},
					},
				},
			},
			match: false,
		},
		{
			name: "expr_exists_true",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: LabelSelectorOpExists,
					},
				},
			},
			match: true,
		},
		{
			name: "expr_exists_false",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "zoo",
						Operator: LabelSelectorOpExists,
					},
				},
			},
			match: false,
		},
		{
			name: "expr_does_not_exist_true",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "zoo",
						Operator: LabelSelectorOpDoesNotExist,
					},
				},
			},
			match: true,
		},
		{
			name: "expr_does_not_exist_false",
			labels: map[string]string{
				"foo": "bar",
			},
			selector: LabelSelector{
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: LabelSelectorOpDoesNotExist,
					},
				},
			},
			match: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.match != tc.selector.Matches(tc.labels) {
				t.Errorf(
					"expected label selector match %v but it didn't",
					tc.match,
				)
			}
		})
	}
}
