// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package smtp

import (
	"context"
	"errors"
	"net/smtp"
	"net/textproto"
	"strings"

	auth_model "code.gitea.io/gitea/models/auth"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/util"
)

// Authenticate queries if the provided login/password is authenticates against the SMTP server
// Users will be autoregistered as required
func (source *Source) Authenticate(ctx context.Context, user *user_model.User, userName, password string) (*user_model.User, error) {
	// Verify allowed domains.
	if len(source.AllowedDomains) > 0 {
		idx := strings.Index(userName, "@")
		if idx == -1 {
			return nil, user_model.ErrUserNotExist{Name: userName}
		} else if !util.SliceContainsString(strings.Split(source.AllowedDomains, ","), userName[idx+1:], true) {
			return nil, user_model.ErrUserNotExist{Name: userName}
		}
	}

	var auth smtp.Auth
	switch source.Auth {
	case PlainAuthentication:
		auth = smtp.PlainAuth("", userName, password, source.Host)
	case LoginAuthentication:
		auth = &loginAuthenticator{userName, password}
	case CRAMMD5Authentication:
		auth = smtp.CRAMMD5Auth(userName, password)
	default:
		return nil, errors.New("unsupported SMTP auth type")
	}

	if err := Authenticate(auth, source); err != nil {
		// when authentication via smtp fails, wraps ErrInvalidArgument
		// with the original textproto.Error as the cause,
		// so it will show username_password_incorrect to the user
		// while log the original error so that admin can check.
		// see: routers/web/auth/auth.go SiginPost
		if tperr, ok := err.(*textproto.Error); ok {
			return nil, errors.Join(util.ErrInvalidArgument, tperr)
		}
		return nil, err
	}

	if user != nil {
		return user, nil
	}

	username := userName
	idx := strings.Index(userName, "@")
	if idx > -1 {
		username = userName[:idx]
	}

	user = &user_model.User{
		LowerName:   strings.ToLower(username),
		Name:        strings.ToLower(username),
		Email:       userName,
		Passwd:      password,
		LoginType:   auth_model.SMTP,
		LoginSource: source.authSource.ID,
		LoginName:   userName,
	}
	overwriteDefault := &user_model.CreateUserOverwriteOptions{
		IsActive: optional.Some(true),
	}

	if err := user_model.CreateUser(ctx, user, &user_model.Meta{}, overwriteDefault); err != nil {
		return user, err
	}

	return user, nil
}

// IsSkipLocalTwoFA returns if this source should skip local 2fa for password authentication
func (source *Source) IsSkipLocalTwoFA() bool {
	return source.SkipLocalTwoFA
}
