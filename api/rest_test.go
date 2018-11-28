package api

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/maddevsio/comedian/chat"
	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/stretchr/testify/assert"
)

func SetUp(SuperAdminID string) *REST {
	c, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}
	if SuperAdminID != "" {
		c.ManagerSlackUserID = SuperAdminID
	}
	slack, err := chat.NewSlack(c)
	if err != nil {
		log.Fatal(err)
	}
	r, err := NewRESTAPI(slack)
	if err != nil {
		log.Fatal(err)
	}
	return r
}

func TestHelpText(t *testing.T) {
	r := SetUp("")
	text := r.displayHelpText()
	assert.Equal(t, "Help Text!", text)
}

func TestAddCommand(t *testing.T) {
	r := SetUp("SuperAdminID")

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
		{2, channel.ChannelID, "<@userID|testUser> / admin", "Users were already assigned as admins: <@userID|testUser>\n"},
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

func TestAddMembers(t *testing.T) {
	r := SetUp("")

	Users := []struct {
		ID   string
		Name string
	}{
		{"userID1", "testUser1"},
		{"userID2", "testUser2"},
		{"userID3", "testUser3"},
	}
	var expectedLine string

	var users []string
	for _, user := range Users {
		user := fmt.Sprintf("<@%v|%v>", user.ID, user.Name)
		users = append(users, user)
		expectedLine += user
	}

	wrongUserData := []string{
		"User1",
		"User2",
	}

	testCase := []struct {
		Users         []string
		RoleInChannel string
		ChannelID     string
		Expected      string
	}{
		{users, "pm", "chan1", "Users are assigned as PMs: " + expectedLine + "\n"},
		{users, "delevoper", "chan1", "Members are assigned: " + expectedLine + "\n"},
		{wrongUserData, "pm", "chan1", "Could not assign users as PMs: User1User2\n"},
		{wrongUserData, "developer", "chan1", "Could not assign members: User1User2\n"},
	}
	for _, test := range testCase {
		actual := r.addMembers(test.Users, test.RoleInChannel, test.ChannelID)
		for _, u := range Users {
			r.db.DeleteChannelMember(u.ID, test.ChannelID)
		}
		assert.Equal(t, test.Expected, actual)
	}
}

func TestListCommand(t *testing.T) {
	//modify test to cover more cases: no users, etc.
	r := SetUp("SuperAdminID")

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

func TestGetAccessLevel(t *testing.T) {
	r := SetUp("SUPERADMINID")

	testCase := []struct {
		UserID    string
		UserName  string
		ChannelID string
		Role      string
		Expected  int
	}{
		{"SUPERADMINID", "SAdminName", "RANDOMCHAN", "", 1},
		{"SUPERADMINID", "SAdminName", "RANDOMCHAN", "pm", 1},
		{"AdminId", "AdminName", "RANDOMCHAN", "admin", 2},
		{"UserId1", "Username", "RANDOMCHAN", "developer", 4},
		{"", "", "", "", 4},
	}

	for _, test := range testCase {
		user, err := r.db.CreateUser(model.User{
			UserID:   test.UserID,
			UserName: test.UserName,
			Role:     test.Role,
		})
		assert.NoError(t, err)
		t.Log("Created: ", user)

		actual, err := r.getAccessLevel(test.UserID, test.ChannelID)
		assert.NoError(t, err)
		assert.Equal(t, test.Expected, actual)

		assert.NoError(t, r.db.DeleteUser(user.ID))
	}

	testCase2 := []struct {
		UserID        string
		UserName      string
		ChannelID     string
		Role          string
		RoleInChannel string
		StandUpTime   int64
		Created       time.Time
		Expected      int
	}{
		{"UserId1", "User1", "ChanId1", "pm", "pm", 1, time.Now(), 3},
		{"UserId2", "User2", "ChanId1", "developer", "pm", 1, time.Now(), 3},
		{"UserId3", "User3", "ChanId1", "admin", "pm", 1, time.Now(), 2},
		{"UserId4", "User4", "ChanId1", "", "designer", 1, time.Now(), 4},
		{"UserId4", "User4", "ChanId1", "", "", 1, time.Now(), 4},
	}

	for _, test := range testCase2 {
		user, err := r.db.CreateUser(model.User{
			UserID:   test.UserID,
			UserName: test.UserName,
			Role:     test.Role,
		})
		assert.NoError(t, err)

		cm, err := r.db.CreateChannelMember(model.ChannelMember{
			UserID:        test.UserID,
			ChannelID:     test.ChannelID,
			RoleInChannel: test.RoleInChannel,
			StandupTime:   test.StandUpTime,
			Created:       test.Created,
		})
		assert.NoError(t, err)

		actual, err := r.getAccessLevel(test.UserID, test.ChannelID)
		assert.NoError(t, err)
		assert.Equal(t, test.Expected, actual)

		assert.NoError(t, r.db.DeleteChannelMember(cm.UserID, cm.ChannelID))
		assert.NoError(t, r.db.DeleteUser(user.ID))
	}
}
