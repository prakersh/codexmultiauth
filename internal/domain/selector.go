package domain

import (
	"fmt"
	"strconv"
	"strings"
)

func ResolveAccount(accounts []Account, selector string) (Account, error) {
	match, err := ResolveAccounts(accounts, selector)
	if err != nil {
		return Account{}, err
	}
	if len(match) != 1 {
		return Account{}, ErrSelectorAmbiguous
	}
	return match[0], nil
}

func ResolveAccounts(accounts []Account, selector string) ([]Account, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, ErrSelectorNotFound
	}
	if strings.EqualFold(selector, "all") {
		return append([]Account(nil), accounts...), nil
	}

	if idx, err := strconv.Atoi(selector); err == nil && idx >= 1 && idx <= len(accounts) {
		return []Account{accounts[idx-1]}, nil
	}

	for _, account := range accounts {
		if account.ID == selector {
			return []Account{account}, nil
		}
	}
	for _, account := range accounts {
		for _, alias := range account.Aliases {
			if alias == selector {
				return []Account{account}, nil
			}
		}
	}
	for _, account := range accounts {
		if account.DisplayName == selector {
			return []Account{account}, nil
		}
	}

	var prefixMatches []Account
	for _, account := range accounts {
		if strings.HasPrefix(account.ID, selector) || strings.HasPrefix(account.DisplayName, selector) {
			prefixMatches = append(prefixMatches, account)
			continue
		}
		for _, alias := range account.Aliases {
			if strings.HasPrefix(alias, selector) {
				prefixMatches = append(prefixMatches, account)
				break
			}
		}
	}
	if len(prefixMatches) == 1 {
		return prefixMatches, nil
	}
	if len(prefixMatches) > 1 {
		return nil, fmt.Errorf("%w: %s", ErrSelectorAmbiguous, selector)
	}
	return nil, fmt.Errorf("%w: %s", ErrSelectorNotFound, selector)
}
