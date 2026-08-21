package account

const (
	CodeUnauthenticated   = "unauthenticated"
	CodeUsernameTaken     = "username_taken"
	CodeInvalidUsername   = "invalid_username"
	CodeInvalidDisplay    = "invalid_display_name"
	CodeEmailImmutable    = "email_immutable"
	CodeNoChanges         = "no_changes"
	CodeAvatarTooLarge    = "avatar_too_large"
	CodeUnsupportedAvatar = "unsupported_avatar"
	CodeInvalidAvatar     = "invalid_avatar"
	CodeBadRequest        = "bad_request"
	CodeInternal          = "internal"
	CodeNetwork           = "network_error"
	CodeServer            = "server_error"
)

type Error struct {
	Code    string
	Field   string
	Status  int
	Message string
	cause   error
}

func (e *Error) Error() string {
	return e.Code
}

func (e *Error) Unwrap() error {
	return e.cause
}
