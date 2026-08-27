package account

const (
	CodeUnauthenticated   = "unauthenticated"
	CodeInvalidLogin      = "invalid_credentials"
	CodeUsernameTaken     = "username_taken"
	CodeEmailTaken        = "email_taken"
	CodeInvalidEmail      = "invalid_email"
	CodeInvalidPassword   = "invalid_password"
	CodeInvalidUsername   = "invalid_username"
	CodeInvalidDisplay    = "invalid_display_name"
	CodeEmailImmutable    = "email_immutable"
	CodeNoChanges         = "no_changes"
	CodeAvatarTooLarge    = "avatar_too_large"
	CodeUnsupportedAvatar = "unsupported_avatar"
	CodeInvalidAvatar     = "invalid_avatar"
	CodeRateLimited       = "rate_limited"
	CodeBadRequest        = "bad_request"
	CodeBlocked           = "request_blocked"
	CodeInternal          = "internal"
	CodeNetwork           = "network_error"
	CodeServer            = "server_error"
	CodeOutdated          = "launcher_outdated"
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
