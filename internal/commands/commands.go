package commands

import (
	"github.com/altid/libs/service/commander"
)

var Commands = []*commander.Command{
	{
		Name:	"action",
		Alias:	[]string{"me", "act"},
		Heading:	commander.ActionGroup,
		Description: "Send an emote to the channel",
	},
	{
		Name:	"msg",
		Alias:	[]string{"query", "m", "q"},
		Args:	[]string{"<name>", "<msg>"},
		Heading:	commander.DefaultGroup,
		Description: "Send a direct message to the user",
	},
}
