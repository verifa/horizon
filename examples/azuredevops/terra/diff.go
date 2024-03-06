package terra

import tfjson "github.com/hashicorp/terraform-json"

func DiffCount(plan *tfjson.Plan) (int, int, int) {
	if plan == nil {
		return 0, 0, 0
	}

	var numCreate, numUpdate, numDelete int
	for _, changes := range plan.ResourceChanges {
		action := changes.Change.Actions
		switch {
		case action.Create():
			numCreate++
		case action.Delete():
			numDelete++
		case action.Update():
			numUpdate++
		case action.DestroyBeforeCreate(), action.CreateBeforeDestroy():
			numCreate++
			numDelete++
		}
	}
	return numCreate, numUpdate, numDelete
}
