package greetings

import (
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
)

const extName = "greetings"

var Portal = hz.Portal{
	ObjectMeta: hz.ObjectMeta{
		Account: hz.RootAccount,
		Name:    extName,
	},
	Spec: hz.PortalSpec{
		DisplayName: "Greetings",
		Icon:        gateway.IconCodeBracketSquare,
	},
}
