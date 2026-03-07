package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/prakersh/codexmultiauth/internal/domain"
)

var askOne = survey.AskOne

func promptText(message, defaultValue string) (string, error) {
	var value string
	err := askOne(&survey.Input{Message: message, Default: defaultValue}, &value)
	return value, err
}

func promptPassword(message string) ([]byte, error) {
	var value string
	err := askOne(&survey.Password{Message: message}, &value)
	return []byte(value), err
}

func promptConfirm(message string, defaultValue bool) (bool, error) {
	var confirmed bool
	err := askOne(&survey.Confirm{Message: message, Default: defaultValue}, &confirmed)
	return confirmed, err
}

func promptMultiSelect(message string, options []string) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}
	var selected []string
	err := askOne(&survey.MultiSelect{Message: message, Options: options}, &selected)
	return selected, err
}

func promptConflictPolicy(accountLabel string) (domain.ConflictPolicy, error) {
	options := []string{
		string(domain.ConflictOverwrite),
		string(domain.ConflictSkip),
		string(domain.ConflictRename),
	}
	var selected string
	err := askOne(&survey.Select{
		Message: fmt.Sprintf("Conflict for %s", accountLabel),
		Options: options,
		Default: string(domain.ConflictOverwrite),
	}, &selected)
	return domain.ConflictPolicy(selected), err
}
