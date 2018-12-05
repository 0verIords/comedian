package api

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/maddevsio/comedian/chat"
	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/maddevsio/comedian/utils"
	"github.com/stretchr/testify/assert"
)

func SetUp() *REST {
	c, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}
	c.ManagerSlackUserID = "SuperAdminID"
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
	r := SetUp()
	text := r.displayHelpText()
	assert.Equal(t, "Help Text!", text)
}

func TestAddCommand(t *testing.T) {
	r := SetUp()

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
	r := SetUp()

	//creates channel member with role pm
	_, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        "testUserId1",
		ChannelID:     "chan1",
		RoleInChannel: "pm",
		Created:       time.Now(),
	})
	assert.NoError(t, err)
	//creates channel member with role developer
	_, err = r.db.CreateChannelMember(model.ChannelMember{
		UserID:        "testUserId2",
		ChannelID:     "chan1",
		RoleInChannel: "dev",
		Created:       time.Now(),
	})
	assert.NoError(t, err)

	testCase := []struct {
		Users         []string
		RoleInChannel string
		Expected      string
	}{
		//existed channel member with role pm
		{[]string{"<@testUserId1|testUserName1>"}, "pm", "Users already have roles: <@testUserId1|testUserName1>\n"},
		//existed channel member with role dev
		{[]string{"<@testUserId2|testUserName2>"}, "dev", "Members already have roles: <@testUserId2|testUserName2>\n"},
		//doesn't existed member with role pm
		{[]string{"<@testUserId3|testUserName3>"}, "pm", "Users are assigned as PMs: <@testUserId3|testUserName3>\n"},
		//two doesn't existed members with role pm
		{[]string{"<@testUserId4|testUserName4>", "<@testUserId5|testUserName5>"}, "pm", "Users are assigned as PMs: <@testUserId4|testUserName4><@testUserId5|testUserName5>\n"},
		//doesn't existed member with role dev
		{[]string{"<@testUserId6|testUserName6>"}, "dev", "Members are assigned: <@testUserId6|testUserName6>\n"},
		//wrong parameters
		{[]string{"user1"}, "pm", "Could not assign users as PMs: user1\n"},
		{[]string{"user1"}, "", "Could not assign members: user1\n"},
		{[]string{"user1", "<>"}, "", "Could not assign members: user1<>\n"},
	}
	for _, test := range testCase {
		actual := r.addMembers(test.Users, test.RoleInChannel, "chan1")
		assert.Equal(t, test.Expected, actual)
	}
	//deletes channelMembers
	for i := 1; i <= 6; i++ {
		err = r.db.DeleteChannelMember(fmt.Sprintf("testUserId%v", i), "chan1")
		assert.NoError(t, err)
	}
}

func TestDeleteCommand(t *testing.T) {
	r := SetUp()

	testCase := []struct {
		accessLevel int
		channelID   string
		params      string
		expected    string
	}{
		{4, "chan1", "<@id|name> / admin", "Access Denied! You need to be at least admin in this slack to use this command!"},
		{4, "chan1", "<@id|name> / pm", "Access Denied! You need to be at least PM in this project to use this command!"},
		{4, "chan1", "<@id|name> / random", "Please, check correct role name (admin, developer, pm)"},
		{4, "chan1", "<@id|name>", "Access Denied! You need to be at least PM in this project to use this command!"},
	}
	for _, test := range testCase {
		actual := r.deleteCommand(test.accessLevel, test.channelID, test.params)
		assert.Equal(t, test.expected, actual)
	}
}

func TestListCommand(t *testing.T) {
	//modify test to cover more cases: no users, etc.
	r := SetUp()

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
	r := SetUp()

	//creates channel without members
	channel1, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChan1",
		ChannelID:   "testChanId1",
	})
	assert.NoError(t, err)
	//creates channel with members
	channel2, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChan2",
		ChannelID:   "testChanId2",
	})
	assert.NoError(t, err)
	//creates channel members
	ChanMem1, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        "userId1",
		ChannelID:     channel2.ChannelID,
		RoleInChannel: "",
		Created:       time.Now(),
	})
	assert.NoError(t, err)

	//parse 10:30 text to int to use it in testCases
	tm, err := utils.ParseTimeTextToInt("10:30")
	assert.NoError(t, err)
	testCase := []struct {
		accessLevel int
		channelID   string
		params      string
		expected    string
	}{
		{4, "", "", "Access Denied! You need to be at least PM in this project to use this command!"},
		{3, channel1.ChannelID, "10:30", fmt.Sprintf("<!date^%v^Standup time at {time} added, but there is no standup users for this channel|Standup time at 12:00 added, but there is no standup users for this channel>", tm)},
		{3, channel2.ChannelID, "10:30", fmt.Sprintf("<!date^%v^Standup time set at {time}|Standup time set at 12:00>", tm)},
		{3, "random", "10:30", fmt.Sprintf("<!date^%v^Standup time at {time} added, but there is no standup users for this channel|Standup time at 12:00 added, but there is no standup users for this channel>", tm)},
	}
	for _, test := range testCase {
		actual := r.addTime(test.accessLevel, test.channelID, test.params)
		assert.Equal(t, test.expected, actual)
	}
	//deletes channels
	err = r.db.DeleteChannel(channel1.ID)
	assert.NoError(t, err)
	err = r.db.DeleteChannel(channel2.ID)
	assert.NoError(t, err)
	//delete channel member
	err = r.db.DeleteChannelMember(ChanMem1.UserID, ChanMem1.ChannelID)
	assert.NoError(t, err)
}

func TestShowTime(t *testing.T) {
	r := SetUp()
	//create a channel with standuptime
	channel1, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChannel1",
		ChannelID:   "testChannelId1",
	})
	assert.NoError(t, err)
	//set a standuptime for channel
	err = r.db.CreateStandupTime(12345, channel1.ChannelID)
	assert.NoError(t, err)
	//create channel without standuptime
	channel2, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChannel2",
		ChannelID:   "testChannelId2",
	})
	assert.NoError(t, err)
	testCase := []struct {
		channelID string
		expected  string
	}{
		{channel1.ChannelID, "<!date^12345^Standup time is {time}|Standup time set at 12:00>"},
		{channel2.ChannelID, "No standup time set for this channel yet! Please, add a standup time using `/standup_time_set` command!"},
		{"doesntExistedChan", "No standup time set for this channel yet! Please, add a standup time using `/standup_time_set` command!"},
	}
	for _, test := range testCase {
		actual := r.showTime(test.channelID)
		assert.Equal(t, test.expected, actual)
	}
	//delete channels
	assert.NoError(t, r.db.DeleteChannel(channel1.ID))
	assert.NoError(t, r.db.DeleteChannel(channel2.ID))
}

func TestShowTimeTable(t *testing.T) {
	r := SetUp()
	//creates channel with members
	channel, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChannel1",
		ChannelID:   "testChannelId1",
		StandupTime: 12345,
	})
	assert.NoError(t, err)
	//adds channel members
	chanMemb1, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        "uid1",
		ChannelID:     channel.ChannelID,
		RoleInChannel: "",
		Created:       time.Now(),
	})
	assert.NoError(t, err)
	//creates timetable for chanMemb1
	timeTable1, err := r.db.CreateTimeTable(model.TimeTable{
		ChannelMemberID: chanMemb1.ID,
		Created:         time.Now(),
		//needs to update timetable
		Modified:  time.Now(),
		Monday:    12345,
		Tuesday:   12345,
		Wednesday: 0,
		Thursday:  0,
		Friday:    12345,
		Saturday:  12345,
		Sunday:    0,
	})
	assert.NoError(t, err)
	//updates timetable
	_, err = r.db.UpdateTimeTable(timeTable1)
	assert.NoError(t, err)
	//creates channel member without timetable
	chanMemb2, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        "uid2",
		ChannelID:     channel.ChannelID,
		RoleInChannel: "",
		Created:       time.Now(),
	})
	assert.NoError(t, err)

	testCases := []struct {
		accessLevel int
		channelID   string
		params      string
		expected    string
	}{
		{2, channel.ChannelID, fmt.Sprintf("<@%v|username1>", chanMemb1.UserID), "Timetable for <@username1> is: | Monday 08:25 | Tuesday 08:25 | Friday 08:25 | Saturday 08:25 |\n"},
		{4, channel.ChannelID, fmt.Sprintf("<@%v|username2>", chanMemb2.UserID), "<@username2> does not have a timetable!\n"},
		{2, channel.ChannelID, "<@randomid|randomName>", "Seems like <@randomName> is not even assigned as standuper in this channel!\n"},
		{4, channel.ChannelID, "wrongParameters", "Seems like you misspelled username. Please, check and try command again!"},
		{2, channel.ChannelID, fmt.Sprintf("<@%v|username1> <@randomid|randomName>", chanMemb1.UserID), "Timetable for <@username1> is: | Monday 08:25 | Tuesday 08:25 | Friday 08:25 | Saturday 08:25 |\nSeems like <@randomName> is not even assigned as standuper in this channel!\n"},
	}
	for _, test := range testCases {
		actual := r.showTimeTable(test.accessLevel, test.channelID, test.params)
		assert.Equal(t, test.expected, actual)
	}
	//deletes timetables
	err = r.db.DeleteTimeTable(timeTable1.ID)
	assert.NoError(t, err)
	//deletes channel members
	err = r.db.DeleteChannelMember(chanMemb1.UserID, channel.ChannelID)
	assert.NoError(t, err)
	err = r.db.DeleteChannelMember(chanMemb2.UserID, channel.ChannelID)
	assert.NoError(t, err)
	//deletes channel
	err = r.db.DeleteChannel(channel.ID)
	assert.NoError(t, err)
}

func TestRemoveTime(t *testing.T) {
	r := SetUp()
	//creates channel with members
	channel1, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChannel1",
		ChannelID:   "testChannelId1",
		StandupTime: int64(100),
	})
	assert.NoError(t, err)
	//adds channel members
	chanMemb1, err := r.db.CreateChannelMember(model.ChannelMember{
		UserID:        "uid1",
		ChannelID:     channel1.ChannelID,
		RoleInChannel: "",
		Created:       time.Now(),
	})
	assert.NoError(t, err)
	//creates channel without members
	channel2, err := r.db.CreateChannel(model.Channel{
		ChannelName: "testChannel2",
		ChannelID:   "testChannelId2",
		StandupTime: int64(100),
	})
	assert.NoError(t, err)

	testCase := []struct {
		accessL  int
		chanID   string
		expected string
	}{
		{4, channel1.ChannelID, "Access Denied! You need to be at least PM in this project to use this command!"},
		{2, channel1.ChannelID, "standup time for this channel removed, but there are people marked as a standuper."},
		{3, channel2.ChannelID, "standup time for channel deleted"},
	}
	for _, test := range testCase {
		actual := r.removeTime(test.accessL, test.chanID)
		assert.Equal(t, test.expected, actual)
	}
	//deletes channels
	assert.NoError(t, r.db.DeleteChannel(channel1.ID))
	assert.NoError(t, r.db.DeleteChannel(channel2.ID))
	//deletes channel members
	err = r.db.DeleteChannelMember(chanMemb1.UserID, chanMemb1.ChannelID)
	assert.NoError(t, err)
}

func TestGetAccessLevel(t *testing.T) {
	r := SetUp()

	testCase := []struct {
		UserID    string
		UserName  string
		ChannelID string
		Role      string
		Expected  int
	}{
		{"SuperAdminID", "SAdminName", "RANDOMCHAN", "", 1},
		{"SuperAdminID", "SAdminName", "RANDOMCHAN", "pm", 1},
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
