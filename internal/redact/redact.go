// Package redact keeps secret material out of anything krmgen prints.
package redact

import (
	"errors"
	"regexp"
	"strings"
)

// urlUserinfo matches the userinfo component of a URL: everything between the
// scheme's "://" and the "@" that closes it.
//
// The character class stops at "/", "?" and "#", so an "@" later in a path
// cannot be mistaken for the end of a userinfo that was never there. It is
// greedy, so a password carrying an unencoded "@" - invalid per RFC 3986, but
// it happens - is masked whole rather than up to its first "@".
var urlUserinfo = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)([^\s/?#]*@)`)

// Credentials masks the password of every URL in s, turning
// "https://user:secret@host" into "https://user:***@host".
//
// A URL carrying only a username is left as it is: a username is not a
// credential, and keeping it is what makes the message worth printing.
func Credentials(s string) string {
	return urlUserinfo.ReplaceAllStringFunc(s, func(match string) string {
		start := strings.Index(match, "://") + len("://")
		userinfo := match[start : len(match)-1] // without the closing "@"
		colon := strings.Index(userinfo, ":")
		if colon < 0 {
			return match
		}
		return match[:start] + userinfo[:colon] + ":***@"
	})
}

// Error returns err with Credentials applied to its message.
//
// A masked error is a new error rather than a wrapper: this is the last stop
// before the message is printed, nothing downstream needs the chain, and
// wrapping would leave the secret one Unwrap away. An error whose message
// holds no credential is returned untouched, so error identity survives the
// common case.
func Error(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	masked := Credentials(message)
	if masked == message {
		return err
	}
	return errors.New(masked)
}
