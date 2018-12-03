package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/schema"
	"github.com/labstack/echo"
	"github.com/maddevsio/comedian/chat"
	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/maddevsio/comedian/reporting"
	"github.com/maddevsio/comedian/storage"
	"github.com/maddevsio/comedian/utils"
	"github.com/sirupsen/logrus"
)

// REST struct used to handle slack requests (slash commands)
type REST struct {
	db     storage.Storage
	echo   *echo.Echo
	conf   config.Config
	report *reporting.Reporter
	slack  *chat.Slack
}

// FullSlackForm struct used for parsing full payload from slack
type FullSlackForm struct {
	Command     string `schema:"command"`
	Text        string `schema:"text"`
	ChannelID   string `schema:"channel_id"`
	ChannelName string `schema:"channel_name"`
	UserID      string `schema:"user_id"`
	UserName    string `schema:"user_name"`
}

// NewRESTAPI creates API for Slack commands
func NewRESTAPI(slack *chat.Slack) (*REST, error) {
	e := echo.New()
	rep := reporting.NewReporter(slack)

	r := &REST{
		echo:   e,
		report: rep,
		db:     slack.DB,
		slack:  slack,
		conf:   slack.Conf,
	}

	r.echo.POST("/commands", r.handleCommands)
	return r, nil
}

//Start starts http server that listens on configured port (default localhost 8080)
func (r *REST) Start() error {
	return r.echo.Start(r.conf.HTTPBindAddr)
}

func (r *REST) handleCommands(c echo.Context) error {
	var form FullSlackForm

	urlValues, err := c.FormParams()
	if err != nil {
		logrus.Errorf("rest: c.FormParams failed: %v\n", err)
		return c.String(http.StatusOK, err.Error())
	}

	decoder := schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)

	if err := decoder.Decode(&form, urlValues); err != nil {
		return c.String(http.StatusOK, err.Error())
	}

	return c.String(http.StatusOK, r.implementCommands(form))

}

func (r *REST) implementCommands(form FullSlackForm) string {
	_, err := r.db.SelectChannel(form.ChannelID)
	if err != nil {
		logrus.Errorf("SelectChannel failed: %v", err)
		return err.Error()
	}

	if form.Command != "/comedian" {
		return err.Error()
	}

	accessLevel, err := r.getAccessLevel(form.UserID, form.ChannelID)
	if err != nil {
		return err.Error()
	}

	command, params := utils.CommandParsing(form.Text)

	switch command {
	case "add":
		return r.addCommand(accessLevel, form.ChannelID, params)
	case "list":
		return r.listCommand(form.ChannelID, params)
	case "delete":
		return r.deleteCommand(accessLevel, form.ChannelID, params)
	case "add_deadline":
		return r.addTime(accessLevel, form.ChannelID, params)
	case "remove_deadline":
		return r.removeTime(accessLevel, form.ChannelID)
	case "show_deadline":
		return r.showTime(form.ChannelID)
	case "add_timetable":
		return r.addTimeTable(accessLevel, form.ChannelID, params)
	case "remove_timetable":
		return r.removeTimeTable(accessLevel, form.ChannelID, params)
	case "show_timetable":
		return r.showTimeTable(accessLevel, form.ChannelID, params)
	case "report_on_user":
		return r.addTimeTable(accessLevel, form.ChannelID, params)
	case "report_on_project":
		return r.removeTimeTable(accessLevel, form.ChannelID, params)
	case "report_on_user_in_project":
		return r.showTimeTable(accessLevel, form.ChannelID, params)
	default:
		return r.displayHelpText()
	}
}

func (r *REST) displayHelpText() string {
	return "Help Text!"
}

func (r *REST) addCommand(accessLevel int, channelID, params string) string {
	var role string
	var members []string
	if strings.Contains(params, "/") {
		dividedText := strings.Split(params, "/")
		role = strings.TrimSpace(dividedText[1])
		members = strings.Fields(dividedText[0])
	} else {
		role = "developer"
		members = strings.Fields(params)
	}

	switch role {
	case "admin", "админ":
		if accessLevel > 2 {
			return r.conf.Translate.AccessAtLeastAdmin
		}
		return r.addAdmins(members)
	case "developer", "разработчик", "":
		if accessLevel > 3 {
			return r.conf.Translate.AccessAtLeastPM
		}
		return r.addMembers(members, "developer", channelID)
	case "pm", "пм":
		if accessLevel > 2 {
			return r.conf.Translate.AccessAtLeastAdmin
		}
		return r.addMembers(members, "pm", channelID)
	default:
		return r.conf.Translate.NeedCorrectUserRole
	}
}

func (r *REST) addMembers(users []string, role, channelID string) string {
	var failed, exist, added, text string

	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")

	for _, u := range users {
		if !rg.MatchString(u) {
			failed += u
			continue
		}
		userID, _ := utils.SplitUser(u)
		user, err := r.db.FindChannelMemberByUserID(userID, channelID)
		if err != nil {
			logrus.Errorf("Rest FindChannelMemberByUserID failed: %v", err)
			chanMember, _ := r.db.CreateChannelMember(model.ChannelMember{
				UserID:        userID,
				ChannelID:     channelID,
				RoleInChannel: role,
			})
			logrus.Infof("ChannelMember created! ID:%v", chanMember.ID)
		}
		if user.UserID == userID && user.ChannelID == channelID {
			exist += u
			continue
		}
		added += u
	}

	if len(failed) != 0 {
		if role == "pm" {
			text += fmt.Sprintf(r.conf.Translate.AddPMsFailed, failed)
		} else {
			text += fmt.Sprintf(r.conf.Translate.AddMembersFailed, failed)
		}
	}
	if len(exist) != 0 {
		if role == "pm" {
			text += fmt.Sprintf(r.conf.Translate.AddPMsExist, exist)
		} else {
			text += fmt.Sprintf(r.conf.Translate.AddMembersExist, exist)
		}

	}
	if len(added) != 0 {
		if role == "pm" {
			text += fmt.Sprintf(r.conf.Translate.AddPMsAdded, added)
		} else {
			text += fmt.Sprintf(r.conf.Translate.AddMembersAdded, added)
		}
	}
	return text
}

func (r *REST) addAdmins(users []string) string {
	var failed, exist, added, text string

	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")

	for _, u := range users {
		if !rg.MatchString(u) {
			failed += u
			continue
		}
		userID, _ := utils.SplitUser(u)
		user, err := r.db.SelectUser(userID)
		if err != nil {
			failed += u
			continue
		}
		if user.Role == "admin" {
			exist += u
			continue
		}
		user.Role = "admin"
		r.db.UpdateUser(user)
		message := r.conf.Translate.PMAssigned
		err = r.slack.SendUserMessage(userID, message)
		if err != nil {
			logrus.Errorf("rest: SendUserMessage failed: %v\n", err)
		}
		added += u
	}

	if len(failed) != 0 {
		text += fmt.Sprintf(r.conf.Translate.AddAdminsFailed, failed)
	}
	if len(exist) != 0 {
		text += fmt.Sprintf(r.conf.Translate.AddAdminsExist, exist)
	}
	if len(added) != 0 {
		text += fmt.Sprintf(r.conf.Translate.AddAdminsAdded, added)
	}

	return text
}

func (r *REST) listCommand(channelID, params string) string {
	switch params {
	case "admin", "админ":
		return r.listAdmins()
	case "developer", "разработчик", "":
		return r.listMembers(channelID, "developer")
	case "pm", "пм":
		return r.listMembers(channelID, "pm")
	default:
		return r.conf.Translate.NeedCorrectUserRole
	}
}

func (r *REST) deleteCommand(accessLevel int, channelID, params string) string {
	var role string
	var members []string
	if strings.Contains(params, "/") {
		dividedText := strings.Split(params, "/")
		role = strings.TrimSpace(dividedText[1])
		members = strings.Fields(dividedText[0])
	} else {
		role = "developer"
		members = strings.Fields(params)
	}

	switch role {
	case "admin", "админ":
		if accessLevel > 2 {
			return r.conf.Translate.AccessAtLeastAdmin
		}
		return r.deleteAdmins(members)
	case "developer", "разработчик", "pm", "пм", "":
		if accessLevel > 3 {
			return r.conf.Translate.AccessAtLeastPM
		}
		return r.deleteMembers(members, channelID)
	default:
		return r.conf.Translate.NeedCorrectUserRole
	}
}

func (r *REST) listMembers(channelID, role string) string {
	members, err := r.db.ListChannelMembersByRole(channelID, role)
	if err != nil {
		return fmt.Sprintf("failed to list members :%v\n", err)
	}
	var userIDs []string
	for _, user := range members {
		userIDs = append(userIDs, "<@"+user.UserID+">")
	}
	if role == "pm" {
		if len(userIDs) < 1 {
			return r.conf.Translate.ListNoPMs
		}
		return fmt.Sprintf(r.conf.Translate.ListPMs, strings.Join(userIDs, ", "))
	}
	if len(userIDs) < 1 {
		return r.conf.Translate.ListNoStandupers
	}
	return fmt.Sprintf(r.conf.Translate.ListStandupers, strings.Join(userIDs, ", "))
}

func (r *REST) listAdmins() string {
	admins, err := r.db.ListAdmins()
	if err != nil {
		return fmt.Sprintf("failed to list users :%v\n", err)
	}
	var userNames []string
	for _, admin := range admins {
		userNames = append(userNames, "<@"+admin.UserName+">")
	}
	if len(userNames) < 1 {
		return r.conf.Translate.ListNoAdmins
	}
	return fmt.Sprintf(r.conf.Translate.ListAdmins, strings.Join(userNames, ", "))
}

func (r *REST) deleteMembers(members []string, channelID string) string {
	var failed, deleted, text string

	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")

	for _, u := range members {
		if !rg.MatchString(u) {
			failed += u
			continue
		}
		userID, _ := utils.SplitUser(u)
		user, err := r.db.FindChannelMemberByUserID(userID, channelID)
		if err != nil {
			logrus.Errorf("rest: FindChannelMemberByUserID failed: %v\n", err)
			failed += u
			continue
		}
		r.db.DeleteChannelMember(user.UserID, channelID)
		deleted += u
	}

	if len(failed) != 0 {
		text += fmt.Sprintf("Could not remove the following members: %v\n", failed)
	}
	if len(deleted) != 0 {
		text += fmt.Sprintf("The following members were removed: %v\n", deleted)
	}

	return text
}

func (r *REST) deleteAdmins(users []string) string {
	var failed, deleted, text string

	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")

	for _, u := range users {
		if !rg.MatchString(u) {
			failed += u
			continue
		}
		userID, _ := utils.SplitUser(u)
		user, err := r.db.SelectUser(userID)
		if err != nil {
			failed += u
			continue
		}
		if user.Role != "admin" {
			failed += u
			continue
		}
		user.Role = ""
		r.db.UpdateUser(user)
		message := fmt.Sprintf(r.conf.Translate.PMRemoved)
		err = r.slack.SendUserMessage(userID, message)
		if err != nil {
			logrus.Errorf("rest: SendUserMessage failed: %v\n", err)
		}
		deleted += u
	}

	if len(failed) != 0 {
		text += fmt.Sprintf("Could not remove users as admins: %v\n", failed)
	}
	if len(deleted) != 0 {
		text += fmt.Sprintf("Users are removed as admins: %v\n", deleted)
	}

	return text
}

func (r *REST) addTime(accessLevel int, channelID, params string) string {
	if accessLevel > 3 {
		return r.conf.Translate.AccessAtLeastPM
	}

	timeInt, err := utils.ParseTimeTextToInt(params)
	if err != nil {
		return err.Error()
	}
	err = r.db.CreateStandupTime(timeInt, channelID)
	if err != nil {
		logrus.Errorf("rest: CreateStandupTime failed: %v\n", err)
		return r.conf.Translate.SomethingWentWrong
	}
	channelMembers, err := r.db.ListChannelMembers(channelID)
	if err != nil {
		logrus.Errorf("rest: ListChannelMembers failed: %v\n", err)
	}
	if len(channelMembers) == 0 {
		return fmt.Sprintf(r.conf.Translate.AddStandupTimeNoUsers, timeInt)
	}
	return fmt.Sprintf(r.conf.Translate.AddStandupTime, timeInt)
}

func (r *REST) showTime(channelID string) string {
	standupTime, err := r.db.GetChannelStandupTime(channelID)
	if err != nil || standupTime == int64(0) {
		logrus.Errorf("GetChannelStandupTime failed: %v", err)
		return r.conf.Translate.ShowNoStandupTime
	}
	return fmt.Sprintf(r.conf.Translate.ShowStandupTime, standupTime)
}

func (r *REST) removeTime(accessLevel int, channelID string) string {
	if accessLevel > 3 {
		return r.conf.Translate.AccessAtLeastPM
	}
	err := r.db.DeleteStandupTime(channelID)
	if err != nil {
		logrus.Errorf("rest: DeleteStandupTime failed: %v\n", err)
		return r.conf.Translate.SomethingWentWrong
	}
	st, err := r.db.ListChannelMembers(channelID)
	if len(st) != 0 {
		return r.conf.Translate.RemoveStandupTimeWithUsers
	}
	return fmt.Sprintf(r.conf.Translate.RemoveStandupTime)
}

func (r *REST) addTimeTable(accessLevel int, channelID, params string) string {
	//add parsing of params
	if accessLevel > 3 {
		return r.conf.Translate.AccessAtLeastPM
	}

	usersText, weekdays, time, err := utils.SplitTimeTalbeCommand(params, r.conf.Translate.DaysDivider, r.conf.Translate.TimeDivider)
	if err != nil {
		return err.Error()
	}
	users := strings.Split(usersText, " ")
	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")
	for _, u := range users {
		if !rg.MatchString(u) {
			logrus.Error(r.conf.Translate.WrongUsernameError)
			continue
		}
		userID, userName := utils.SplitUser(u)

		m, err := r.db.FindChannelMemberByUserID(userID, channelID)
		if err != nil {
			m, err = r.db.CreateChannelMember(model.ChannelMember{
				UserID:    userID,
				ChannelID: channelID,
			})
			if err != nil {
				continue
			}
		}

		tt, err := r.db.SelectTimeTable(m.ID)
		if err != nil {
			logrus.Infof("Timetable for this standuper does not exist. Creating...")
			ttNew, err := r.db.CreateTimeTable(model.TimeTable{
				ChannelMemberID: m.ID,
			})
			ttNew = utils.PrepareTimeTable(ttNew, weekdays, time)
			ttNew, err = r.db.UpdateTimeTable(ttNew)
			if err != nil {
				fmt.Sprintf(r.conf.Translate.CanNotUpdateTimetable, userName, err)
				continue
			}
			logrus.Infof("Timetable created id:%v", ttNew.ID)
			fmt.Sprintf(r.conf.Translate.TimetableCreated, userID, ttNew.Show())
			continue
		}
		tt = utils.PrepareTimeTable(tt, weekdays, time)
		tt, err = r.db.UpdateTimeTable(tt)
		if err != nil {
			fmt.Sprintf(r.conf.Translate.CanNotUpdateTimetable, userName, err)
			continue
		}
		logrus.Infof("Timetable updated id:%v", tt.ID)
		fmt.Sprintf(r.conf.Translate.TimetableUpdated, userID, tt.Show())
	}
	return ""
}

func (r *REST) showTimeTable(accessLevel int, channelID, params string) string {
	var totalString string
	//add parsing of params
	users := strings.Split(params, " ")
	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")
	for _, u := range users {
		if !rg.MatchString(u) {
			logrus.Error(r.conf.Translate.WrongUsernameError)
			continue
		}
		userID, userName := utils.SplitUser(u)

		m, err := r.db.FindChannelMemberByUserID(userID, channelID)
		if err != nil {
			totalString += fmt.Sprintf(r.conf.Translate.NotAStanduper, userName)
			continue
		}
		tt, err := r.db.SelectTimeTable(m.ID)
		if err != nil {
			totalString += fmt.Sprintf(r.conf.Translate.NoTimetableSet, userName)
			continue
		}
		totalString += fmt.Sprintf(r.conf.Translate.TimetableShow, userName, tt.Show())
	}
	return totalString
}

func (r *REST) removeTimeTable(accessLevel int, channelID, params string) string {
	//add parsing of params

	if accessLevel > 3 {
		return r.conf.Translate.AccessAtLeastPM
	}

	users := strings.Split(params, " ")
	rg, _ := regexp.Compile("<@([a-z0-9]+)|([a-z0-9]+)>")
	for _, u := range users {
		if !rg.MatchString(u) {
			logrus.Error(r.conf.Translate.WrongUsernameError)
			continue
		}
		userID, userName := utils.SplitUser(u)

		m, err := r.db.FindChannelMemberByUserID(userID, channelID)
		if err != nil {
			fmt.Sprintf(r.conf.Translate.NotAStanduper, userName)
			continue
		}
		tt, err := r.db.SelectTimeTable(m.ID)
		if err != nil {
			fmt.Sprintf(r.conf.Translate.NoTimetableSet, userName)
			continue
		}
		err = r.db.DeleteTimeTable(tt.ID)
		if err != nil {
			fmt.Sprintf(r.conf.Translate.CanNotDeleteTimetable, userName)
			continue
		}
		fmt.Sprintf(r.conf.Translate.TimetableDeleted, userName)
	}
	return ""
}

func (r *REST) reportByProject(accessLevel int, channelID, params string) string {

	if accessLevel > 3 {
		return r.conf.Translate.AccessAtLeastPM
	}

	commandParams := strings.Fields(params)
	if len(commandParams) != 3 {
		return r.conf.Translate.WrongNArgs
	}
	channelName := strings.Replace(commandParams[0], "#", "", -1)
	channelID, err := r.db.GetChannelID(channelName)
	if err != nil {
		logrus.Errorf("rest: GetChannelID failed: %v\n", err)
		return "Неверное название проекта!"
	}

	channel, err := r.db.SelectChannel(channelID)
	if err != nil {
		logrus.Errorf("rest: SelectChannel failed: %v\n", err)
		return err.Error()
	}

	dateFrom, err := time.Parse("2006-01-02", commandParams[1])
	if err != nil {
		logrus.Errorf("rest: time.Parse failed: %v\n", err)
		return err.Error()
	}
	dateTo, err := time.Parse("2006-01-02", commandParams[2])
	if err != nil {
		logrus.Errorf("rest: time.Parse failed: %v\n", err)
		return err.Error()
	}

	report, err := r.report.StandupReportByProject(channel, dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("rest: StandupReportByProject: %v\n", err)
		return err.Error()
	}

	text := ""
	text += report.ReportHead
	if len(report.ReportBody) == 0 {
		text += r.conf.Translate.ReportNoData
		return text
	}
	for _, t := range report.ReportBody {
		text += t.Text
	}
	return text
}

func (r *REST) reportByUser(accessLevel int, channelID, params string) string {

	commandParams := strings.Fields(params)
	if len(commandParams) != 3 {
		return r.conf.Translate.WrongNArgs
	}
	username := strings.Replace(commandParams[0], "@", "", -1)
	user, err := r.db.SelectUserByUserName(username)
	if err != nil {
		return "User does not exist!"
	}

	//if f.Get("user_id") != user.UserID && accessLevel > 2 { was removed, need to fix bug
	if accessLevel > 2 {
		return r.conf.Translate.AccessAtLeastAdminOrOwner
	}

	dateFrom, err := time.Parse("2006-01-02", commandParams[1])
	if err != nil {
		logrus.Errorf("rest: time.Parse failed: %v\n", err)
		return err.Error()
	}
	dateTo, err := time.Parse("2006-01-02", commandParams[2])
	if err != nil {
		logrus.Errorf("rest: time.Parse failed: %v\n", err)
		return err.Error()
	}

	report, err := r.report.StandupReportByUser(user.UserID, dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("rest: StandupReportByUser failed: %v\n", err)
		return err.Error()
	}

	text := ""
	text += report.ReportHead
	if len(report.ReportBody) == 0 {
		text += r.conf.Translate.ReportNoData
		return text
	}
	for _, t := range report.ReportBody {
		text += t.Text
	}
	return text
}

func (r *REST) reportByProjectAndUser(accessLevel int, channelID, params string) string {

	commandParams := strings.Fields(params)
	if len(commandParams) != 4 {
		return r.conf.Translate.WrongNArgs
	}

	channelName := strings.Replace(commandParams[0], "#", "", -1)
	channelID, err := r.db.GetChannelID(channelName)
	if err != nil {
		logrus.Errorf("rest: GetChannelID failed: %v\n", err)
		return r.conf.Translate.WrongProjectName
	}

	channel, err := r.db.SelectChannel(channelID)
	if err != nil {
		logrus.Errorf("rest: SelectChannel failed: %v\n", err)
		return err.Error()
	}

	username := strings.Replace(commandParams[1], "@", "", -1)

	user, err := r.db.SelectUserByUserName(username)
	if err != nil {
		return r.conf.Translate.NoSuchUserInWorkspace
	}
	member, err := r.db.FindChannelMemberByUserName(user.UserName, channelID)
	if err != nil {
		return fmt.Sprintf(r.conf.Translate.CanNotFindMember, user.UserID)
	}

	// if (f.Get("user_id") != member.UserID && channelID != member.ChannelID) && accessLevel > 3 {
	// 	return r.conf.Translate.AccessAtLeastPMOrOwner
	// } Need to fix bug!

	if accessLevel > 3 {
		return r.conf.Translate.AccessAtLeastPMOrOwner
	}

	dateFrom, err := time.Parse("2006-01-02", commandParams[2])
	if err != nil {
		logrus.Errorf("rest: time.Parse failed: %v\n", err)
		return err.Error()
	}
	dateTo, err := time.Parse("2006-01-02", commandParams[3])
	if err != nil {
		logrus.Errorf("rest: time.Parse failed: %v\n", err)
		return err.Error()
	}

	report, err := r.report.StandupReportByProjectAndUser(channel, member.UserID, dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("rest: StandupReportByProjectAndUser failed: %v\n", err)
		return err.Error()
	}

	text := ""
	text += report.ReportHead
	if len(report.ReportBody) == 0 {
		text += r.conf.Translate.ReportNoData
		return text
	}
	for _, t := range report.ReportBody {
		text += t.Text
	}
	return text
}

func (r *REST) getAccessLevel(userID, channelID string) (int, error) {
	user, err := r.db.SelectUser(userID)
	if err != nil {
		return 0, err
	}
	if userID == r.conf.ManagerSlackUserID {
		return 1, nil
	}
	if user.IsAdmin() {
		return 2, nil
	}
	if r.db.UserIsPMForProject(userID, channelID) {
		return 3, nil
	}
	return 4, nil
}
