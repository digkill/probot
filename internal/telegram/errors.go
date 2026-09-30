package telegram

import (
	"context"
	"errors"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tgerr"
)

func rpcError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var own *SafeError
	if errors.As(err, &own) {
		return err
	}
	if tgerr.Is(err, "PHONE_CODE_INVALID", "PHONE_CODE_EXPIRED", "PHONE_CODE_EMPTY") {
		return safe(ErrInvalidCode, err)
	}
	if errors.Is(err, auth.ErrPasswordAuthNeeded) || tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
		return safe(ErrPasswordRequired, err)
	}
	if errors.Is(err, auth.ErrPasswordInvalid) || tgerr.Is(err, "PASSWORD_HASH_INVALID") {
		return safe(ErrInvalidPassword, err)
	}
	if auth.IsUnauthorized(err) || tgerr.Is(err, "AUTH_KEY_UNREGISTERED", "AUTH_KEY_INVALID", "AUTH_KEY_DUPLICATED", "SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED", "USER_DEACTIVATED_BAN") {
		return safe(ErrSessionInvalid, err)
	}
	if _, ok := tgerr.AsFloodWait(err); ok {
		return safe(ErrFloodWait, err)
	}
	if tgerr.Is(err, "USERNAME_NOT_OCCUPIED", "USERNAME_INVALID", "PEER_ID_INVALID", "CHANNEL_INVALID") {
		return safe(ErrPeerNotFound, err)
	}
	return safe(ErrUnavailable, err)
}
