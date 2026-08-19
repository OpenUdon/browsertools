package bundle

import "errors"

var (
	ErrValidation = errors.New("bundle validation failure")
	ErrExpired    = errors.New("bundle expiry failure")
	ErrPolicy     = errors.New("bundle policy failure")
	ErrIntegrity  = errors.New("bundle integrity failure")
	ErrLimit      = errors.New("bundle limit failure")
	ErrConflict   = errors.New("bundle conflict")
)

func categorized(err, fallback error) error {
	if err == nil {
		return nil
	}
	for _, category := range []error{ErrValidation, ErrExpired, ErrPolicy, ErrIntegrity, ErrLimit, ErrConflict} {
		if errors.Is(err, category) {
			return err
		}
	}
	return errors.Join(fallback, err)
}
