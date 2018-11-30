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

func TestDeleteCommand(t *testing.T) {
	r := SetUp("")

	Params := []struct {
		ID   string
		Name string
		Role string
	}{
		{"UserID1", "User1", "developer"},
		{"UserID2", "User2", "разработчик"},
		{"UserID3", "User3", "pm"},
		{"UserID4", "User4", "пм"},
		{"UserID5", "User5", ""},
		{"UserID6", "User6", "admin"},
		{"UserID7", "User7", "админ"},
		{"UserID8", "User8", "randomRole"},
	}
	var ParamString string
	ParamStrings := make(map[string]string)
	for _, p := range Params {
		ParamString = fmt.Sprintf("<@%v|%v>/%v", p.ID, p.Name, p.Role)
		ParamStrings[p.Role] = ParamString
	}

	testCase := []struct {
		AccessL   int
		ChannelID string
		Params    string
		Expected  string
	}{
		{4, "CHAN1", ParamStrings["developer"], "Access Denied! You need to be at least PM in this project to use this command!"},
		{4, "CHAN1", ParamStrings["разработчик"], "Access Denied! You need to be at least PM in this project to use this command!"},
		{4, "CHAN1", ParamStrings["pm"], "Access Denied! You need to be at least PM in this project to use this command!"},
		{4, "CHAN1", ParamStrings["пм"], "Access Denied! You need to be at least PM in this project to use this command!"},
		{4, "CHAN1", ParamStrings[""], "Access Denied! You need to be at least PM in this project to use this command!"},
		{4, "CHAN1", ParamStrings["admin"], "Access Denied! You need to be at least admin in this slack to use this command!"},
		{4, "CHAN1", ParamStrings["админ"], "Access Denied! You need to be at least admin in this slack to use this command!"},
		//accessLevel 3 can delete user pm,but userID3(pm) don'exist in db
		{3, "CHAN1", ParamStrings["pm"], "Could not remove the following members: <@UserID3|User3>\n"},
		{4, "CHAN1", ParamStrings["randomRole"], "Please, check correct role name (admin, developer, pm)"},
	}

	for _, test := range testCase {
		actual := r.deleteCommand(test.AccessL, test.ChannelID, test.Params)
		assert.Equal(t, test.Expected, actual)
	}

	var WrongParamString string
	WrongParamStrings := make(map[string]string)
	for _, p := range Params {
		WrongParamString = fmt.Sprintf("<@%v |%v>//%v", p.ID, p.Name, p.Role)
		WrongParamStrings[p.Role] = WrongParamString
	}
	testCase2 := []struct {
		AccessL   int
		ChannelID string
		Params    string
		Expected  string
	}{
		{4, "CHAN1", WrongParamStrings["developer"], "Access Denied! You need to be at least PM in this project to use this command!"},
		{3, "CHAN1", WrongParamStrings["developer"], "Could not remove the following members: <@UserID1|User1>\n"},
		{3, "CHAN1", WrongParamStrings["admin"], "Could not remove the following members: <@UserID6|User6>\n"},
	}
	for _, test := range testCase2 {
		actual := r.deleteCommand(test.AccessL, test.ChannelID, test.Params)
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

func TestAddTime(t *testing.T) {
	r := SetUp("")
	channel, err := r.db.CreateChannel(model.Channel{
		ChannelName: "chan1",
		ChannelID:   "chanId1",
		StandupTime: int64(0),
	})
	assert.NoError(t, err)
	channel2, err := r.db.CreateChannel(model.Channel{
		ChannelName: "chan1",
		ChannelID:   "chanId2",
		StandupTime: int64(0),
	})
	assert.NoError(t, err)
	chanMemb := []struct {
		UserID     string
		ChannelID  string
		RoleInChan string
		Created    time.Time
	}{
		{"uid1", "chanId1", "", time.Now()},
		{"uid2", "chanId1", "", time.Now()},
	}

	for _, cm := range chanMemb {
		r.db.CreateChannelMember(model.ChannelMember{
			UserID:        cm.UserID,
			ChannelID:     cm.ChannelID,
			RoleInChannel: cm.RoleInChan,
			Created:       cm.Created,
		})
	}

	testCase := []struct {
		accessL   int
		channelID string
		params    string
		expected  string
	}{
		{4, "chanId1", "params", "Access Denied! You need to be at least PM in this project to use this command!"},
		{2, "randomChanId", "12345", "Could not understand how you mention time. Please, use 24:00 hour format and try again!"},
		{2, "chanId2", "10:30", "<!date^1543552200^Standup time at {time} added, but there is no standup users for this channel|Standup time at 12:00 added, but there is no standup users for this channel>"},
		{2, "chanId1", "10:30", "<!date^1543552200^Standup time set at {time}|Standup time set at 12:00>"},
		{2, "", "", "Could not understand how you mention time. Please, use 24:00 hour format and try again!"},
		//in this case channel doesn't exist,but expected line must contain "...there is no standup users for this channel..."
		//this is not an error,because checking existance of channel not in this function
		{2, "", "10:30", "<!date^1543552200^Standup time at {time} added, but there is no standup users for this channel|Standup time at 12:00 added, but there is no standup users for this channel>"},
	}
	for _, test := range testCase {
		actual := r.addTime(test.accessL, test.channelID, test.params)
		assert.Equal(t, test.expected, actual)
	}
	for _, cm := range chanMemb {
		assert.NoError(t, r.db.DeleteChannelMember(cm.UserID, cm.ChannelID))
	}
	assert.NoError(t, r.db.DeleteChannel(channel.ID))
	assert.NoError(t, r.db.DeleteChannel(channel2.ID))
}

func TestRemoveTime(t *testing.T) {
	r := SetUp("")
	channel, err := r.db.CreateChannel(model.Channel{
		ChannelName: "chan1",
		ChannelID:   "chanId1",
		StandupTime: int64(100),
	})
	assert.NoError(t, err)
	channelWithMembers, err := r.db.CreateChannel(model.Channel{
		ChannelName: "chan2",
		ChannelID:   "chanId2",
		StandupTime: int64(100),
	})
	assert.NoError(t, err)
	//add members to this channel
	chanMemb := []struct {
		UserID     string
		ChannelID  string
		RoleInChan string
		Created    time.Time
	}{
		{"uid1", "chanId2", "", time.Now()},
		{"uid2", "chanId2", "", time.Now()},
	}
	for _, cm := range chanMemb {
		r.db.CreateChannelMember(model.ChannelMember{
			UserID:        cm.UserID,
			ChannelID:     cm.ChannelID,
			RoleInChannel: cm.RoleInChan,
			Created:       cm.Created,
		})
	}

	testCase := []struct {
		accessL  int
		chanID   string
		expected string
	}{
		{4, "chanID1", "Access Denied! You need to be at least PM in this project to use this command!"},
		{2, "chanId1", "standup time for chanId1 channel deleted"},
		{2, "chanId2", "standup time for this channel removed, but there are people marked as a standuper."},
	}
	for _, test := range testCase {
		actual := r.removeTime(test.accessL, test.chanID)
		assert.Equal(t, test.expected, actual)
	}
	assert.NoError(t, r.db.DeleteChannel(channel.ID))
	assert.NoError(t, r.db.DeleteChannel(channelWithMembers.ID))
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
