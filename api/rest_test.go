package api

import (
	"testing"

	"github.com/maddevsio/comedian/chat"
	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/stretchr/testify/assert"
)

func TestHelpText(t *testing.T) {
	c, err := config.Get()
	assert.NoError(t, err)
	c.ManagerSlackUserID = "SuperAdminID"
	slack, err := chat.NewSlack(c)
	assert.NoError(t, err)
	r, err := NewRESTAPI(slack)
	assert.NoError(t, err)

	text := r.displayHelpText()
	assert.Equal(t, "Help Text!", text)

}
func TestAddCommand(t *testing.T) {
	c, err := config.Get()
	assert.NoError(t, err)
	c.ManagerSlackUserID = "SuperAdminID"
	slack, err := chat.NewSlack(c)
	assert.NoError(t, err)
	r, err := NewRESTAPI(slack)
	assert.NoError(t, err)

	user, err := r.db.CreateUser(model.User{
		UserName: "testUser",
		UserID:   "userID",
		Role:     "",
	})
	assert.NoError(t, err)

	channel, err := r.db.CreateChannel(model.Channel{
		ChannelName: "TestChannel",
		ChannelID:   "TestChannelID",
		StandupTime: int64(0),
	})
	assert.NoError(t, err)

	testCases := []struct {
		accessLevel int
		channelID   string
		params      string
		output      string
	}{
		{1, channel.ChannelID, "<@userID|testUser>", "Members are assigned: <@userID|testUser>\n"},
		{4, channel.ChannelID, "<@userID|testUser>", "Access Denied! You need to be at least PM in this project to use this command!"},
		{2, channel.ChannelID, "<@userID|testUser> / admin", "Users are assigned as admins: <@userID|testUser>\n"},
		{3, channel.ChannelID, "<@userID|testUser> / admin", "Access Denied! You need to be at least admin in this slack to use this command!"},
		{2, channel.ChannelID, "<@userID|testUser> / pm", "Users are assigned as PMs: <@userID|testUser>\n"},
		{3, channel.ChannelID, "<@userID|testUser> / pm", "Access Denied! You need to be at least admin in this slack to use this command!"},
		{2, channel.ChannelID, "<@userID|testUser> / wrongRole", "Please, check correct role name (admin, developer, pm)"},
	}

	for _, tt := range testCases {
		result := r.addCommand(tt.accessLevel, tt.channelID, tt.params)
		assert.Equal(t, tt.output, result)

		members, err := r.db.ListAllChannelMembers()
		assert.NoError(t, err)
		for _, m := range members {
			assert.NoError(t, r.db.DeleteChannelMember(m.UserID, m.ChannelID))
		}
	}

	assert.NoError(t, r.db.DeleteChannel(channel.ID))
	assert.NoError(t, r.db.DeleteUser(user.ID))
}

func TestListCommand(t *testing.T) {
	//modify test to cover more cases: no users, etc.
	c, err := config.Get()
	assert.NoError(t, err)
	c.ManagerSlackUserID = "SuperAdminID"
	slack, err := chat.NewSlack(c)
	assert.NoError(t, err)
	r, err := NewRESTAPI(slack)
	assert.NoError(t, err)

	channel, err := r.db.CreateChannel(model.Channel{
		ChannelName: "TestChannel",
		ChannelID:   "TestChannelID",
		StandupTime: int64(0),
	})
	assert.NoError(t, err)

	user, err := r.db.CreateUser(model.User{
		UserName: "testUser",
		UserID:   "userID",
		Role:     "",
	})
	assert.NoError(t, err)

	admin, err := r.db.CreateUser(model.User{
		UserName: "testUser",
		UserID:   "userID",
		Role:     "admin",
	})
	assert.NoError(t, err)

	memberPM, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        user.UserID,
		ChannelID:     channel.ChannelID,
		RoleInChannel: "pm",
	})

	memberDeveloper, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        user.UserID,
		ChannelID:     channel.ChannelID,
		RoleInChannel: "developer",
	})

	testCases := []struct {
		params string
		output string
	}{
		{"", "Standupers in this channel: <@userID>"},
		{"admin", "Admins in this workspace: <@testUser>"},
		{"developer", "Standupers in this channel: <@userID>"},
		{"pm", "PMs in this channel: <@userID>"},
		{"randomRole", "Please, check correct role name (admin, developer, pm)"},
	}

	for _, tt := range testCases {
		result := r.listCommand(channel.ChannelID, tt.params)
		assert.Equal(t, tt.output, result)
	}

	assert.NoError(t, r.db.DeleteChannel(channel.ID))
	assert.NoError(t, r.db.DeleteUser(user.ID))
	assert.NoError(t, r.db.DeleteUser(admin.ID))
	assert.NoError(t, r.db.DeleteChannelMember(memberPM.UserID, memberPM.ChannelID))
	assert.NoError(t, r.db.DeleteChannelMember(memberDeveloper.UserID, memberDeveloper.ChannelID))
}
