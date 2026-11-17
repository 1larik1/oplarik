//go:build windows

package main

import (
	"fmt"
	"os"
)

// registerTask creates (or replaces) a Task Scheduler task that runs Larik
// with args as the signed-in user, with their highest rights (no UAC prompt).
func registerTask(name, desc, triggers, args string, startWhenAvailable bool, limit string) error {
	ws := &winSys{}
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	swa := "false"
	if startWhenAvailable {
		swa = "true"
	}
	xml := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>` + xmlEsc(desc) + `</Description></RegistrationInfo>
  <Triggers>` + triggers + `</Triggers>
  <Principals><Principal id="Author"><UserId>` + xmlEsc(user) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>` + swa + `</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <ExecutionTimeLimit>` + limit + `</ExecutionTimeLimit>
    <Priority>6</Priority>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + xmlEsc(overlayTaskExe()) + `</Command><Arguments>` + xmlEsc(args) + `</Arguments></Exec></Actions>
</Task>`
	f, err := os.CreateTemp("", "larik-task-*.xml")
	if err != nil {
		return err
	}
	data := utf16Bytes(xml)
	_, _ = f.Write(append([]byte{0xFF, 0xFE}, data[:len(data)-2]...))
	f.Close()
	defer os.Remove(f.Name())
	_, err = ws.Exec("schtasks", "/Create", "/TN", name, "/XML", f.Name(), "/F")
	return err
}

func deleteTask(name string) {
	_, _ = (&winSys{}).Exec("schtasks", "/Delete", "/TN", name, "/F")
}

var weekDays = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// syncCleanTask creates or removes the weekly cleanup task.
func syncCleanTask(c AutoCleanConfig) error {
	const name = "Larik Cleanup"
	if !c.Enabled {
		deleteTask(name)
		return nil
	}
	c.Normalize()
	trig := fmt.Sprintf(`<CalendarTrigger><StartBoundary>2026-01-04T%02d:00:00</StartBoundary><Enabled>true</Enabled>`+
		`<ScheduleByWeek><DaysOfWeek><%s/></DaysOfWeek><WeeksInterval>1</WeeksInterval></ScheduleByWeek></CalendarTrigger>`, c.Hour, weekDays[c.Day])
	return registerTask(name, "OpLarik weekly cleanup of temporary files", trig, "--clean", true, "PT1H")
}
