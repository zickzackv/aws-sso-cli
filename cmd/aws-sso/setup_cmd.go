package main

/*
 * AWS SSO CLI
 * Copyright (c) 2021-2023 Aaron Turner  <synfinatic at gmail dot com>
 *
 * This program is free software: you can redistribute it
 * and/or modify it under the terms of the GNU General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or with the authors permission any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/synfinatic/aws-sso-cli/internal/url"
	"github.com/synfinatic/aws-sso-cli/internal/utils"
	"github.com/synfinatic/aws-sso-cli/sso"
)

var ranSetup = false

// SetupCmd defines the Kong args for the setup command (which currently doesn't exist)
type SetupCmd struct{}

// Run executes the setup command
func (cc *SetupCmd) Run(ctx *RunContext) error {
	return setupWizard(ctx, false, false)
}

type ConfigCmd struct {
	// 	AddSSO bool `kong:"help='Add a new AWS SSO instance'"`
}

func (cc *ConfigCmd) Run(ctx *RunContext) error {
	if err := backupConfig(ctx.Cli.ConfigFile); err != nil {
		return err
	}

	return setupWizard(ctx, true, false) // ctx.Cli.Config.AddSSO)
}

func setupWizard(ctx *RunContext, reconfig, addSSO bool) error {
	var s = ctx.Settings

	// Don't run setup twice
	if ranSetup {
		return nil
	}
	ranSetup = true

	fmt.Printf(`
**********************************************************************
* Do you have questions?  Do you like reading docs?  We've got docs! *
* https://github.com/synfinatic/aws-sso-cli/blob/main/docs/config.md *
**********************************************************************

`)

	if reconfig {
		// migrate old boolean flag to enum
		if s.FirefoxOpenUrlInContainer {
			s.UrlAction = url.OpenUrlContainer
		}

		// upgrade deprecated config option
		if s.ConfigUrlAction != "" && s.ConfigProfilesUrlAction == "" {
			s.ConfigProfilesUrlAction, _ = url.NewConfigProfilesAction(s.ConfigUrlAction)
			s.ConfigUrlAction = ""
		}
		// skips:
		// - SSORegion
		// - DefaultRegion
		// - StartUrl/startHostname
		// - InstanceName
	} else {
		instanceName := promptSsoInstance("")
		startHostname := promptStartUrl("")
		ssoRegion := promptAwsSsoRegion("")
		defaultRegion := promptDefaultRegion(ssoRegion)

		s = &sso.Settings{
			SSO:             map[string]*sso.SSOConfig{},
			UrlAction:       "open",
			LogLevel:        "error",
			DefaultRegion:   defaultRegion,
			ConsoleDuration: 60,
			CacheRefresh:    168,
			AutoConfigCheck: false,
			FullTextSearch:  true,
			HistoryLimit:    10,
			HistoryMinutes:  1440,
			UrlExecCommand:  []string{},
		}

		s.SSO[instanceName] = &sso.SSOConfig{
			SSORegion:     ssoRegion,
			StartUrl:      fmt.Sprintf(START_URL_FORMAT, startHostname),
			DefaultRegion: defaultRegion,
		}
	}

	// first, caching
	s.CacheRefresh = promptCacheRefresh(s.CacheRefresh)

	if s.CacheRefresh > 0 {
		s.AutoConfigCheck = promptAutoConfigCheck(s.AutoConfigCheck)
	}

	// full text search?
	s.FullTextSearch = promptFullTextSearch(s.FullTextSearch)

	// next how we open URLs
	s.UrlAction = promptUrlAction(s.UrlAction)
	s.ConfigProfilesUrlAction = promptConfigProfilesUrlAction(s.ConfigProfilesUrlAction, s.UrlAction)

	// do we need urlExecCommand?
	if s.UrlAction == url.Exec {
		s.UrlExecCommand = promptUrlExecCommand(s.UrlExecCommand)
	} else if s.UrlAction.IsContainer() {
		s.UrlExecCommand = promptUseFirefox(s.UrlExecCommand)
	} else {
		s.UrlExecCommand = []string{}
	}

	// should we prompt user to override default browser?
	if s.UrlAction == url.Open || s.ConfigProfilesUrlAction == url.ConfigProfilesOpen {
		s.Browser = promptDefaultBrowser(s.Browser)
	}

	s.ConsoleDuration = promptConsoleDuration(s.ConsoleDuration)
	s.HistoryLimit = promptHistoryLimit(s.HistoryLimit)
	s.HistoryMinutes = promptHistoryMinutes(s.HistoryMinutes)
	s.LogLevel = promptLogLevel(s.LogLevel)

	if err := s.Validate(); err != nil {
		return err
	}

	fmt.Printf("\nAwesome!  Saving the new %s\n", ctx.Cli.ConfigFile)
	return s.Save(ctx.Cli.ConfigFile, reconfig)
}

// backupConfig copies the specified config file to its backup
func backupConfig(cfgFile string) error {
	var i int

	// only backup file if it exists
	if _, err := os.Open(cfgFile); err == nil {
		label := fmt.Sprintf("Backup %s first?", cfgFile)
		sel := promptui.Select{
			Label:        label,
			Items:        yesNoItems,
			CursorPos:    yesNoPos(true),
			HideSelected: false,
			Stdout:       &utils.BellSkipper{},
			Templates:    makeSelectTemplate(label),
		}
		if i, _, err = sel.Run(); err != nil {
			return err
		}

		// user said yes
		if yesNoItems[i].Value == "Yes" {
			sourcePath := utils.GetHomePath(cfgFile)
			src, err := os.Open(sourcePath)
			if err != nil {
				return err
			}

			dir := path.Dir(sourcePath)
			fileName := path.Base(sourcePath)
			fileparts := strings.Split(fileName, ".")
			ext := "yaml"
			if len(fileparts) > 1 {
				ext = fileparts[len(fileparts)-1]
				fileparts = fileparts[:len(fileparts)-1]
			}

			fileparts = append(fileparts, time.Now().Format("2006-01-02-15:04:05"))
			fileparts = append(fileparts, ext)

			newFile := strings.Join(fileparts, ".")
			newFile = path.Join(dir, newFile)

			dst, err := os.Create(newFile)
			if err != nil {
				return err
			}
			if _, err = io.Copy(dst, src); err != nil {
				return err
			}

			src.Close()
			dst.Close()
			fmt.Printf("Wrote: %s\n\n", newFile)
		}
	}

	return nil
}
