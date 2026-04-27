package app

import "errors"

type invalidInputError struct {
	message string
}

func (e invalidInputError) Error() string {
	if e.message == "" {
		return ErrInvalidInput.Error()
	}
	return ErrInvalidInput.Error() + ": " + e.message
}

func (e invalidInputError) Unwrap() error {
	return ErrInvalidInput
}

func invalidInput(message string) error {
	return invalidInputError{message: message}
}

func InvalidInputMessage(err error) string {
	var inputErr invalidInputError
	if errors.As(err, &inputErr) && inputErr.message != "" {
		return inputErr.message
	}
	return ErrInvalidInput.Error()
}
