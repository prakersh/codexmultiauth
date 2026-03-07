package domain

import "errors"

var (
	ErrSelectorNotFound  = errors.New("selector not found")
	ErrSelectorAmbiguous = errors.New("selector is ambiguous")
	ErrAccountExists     = errors.New("account already exists")
)
