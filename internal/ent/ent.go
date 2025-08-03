// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package ent

import (
	"strings"
	"unicode"

	"github.com/go-openapi/inflect"
)

var (
	rules    = ruleset()
	acronyms = make(map[string]struct{})
)

func pascalWords(words []string) string {
	for i, w := range words {
		upper := strings.ToUpper(w)
		if _, ok := acronyms[upper]; ok {
			words[i] = upper
		} else {
			words[i] = rules.Capitalize(w)
		}
	}
	return strings.Join(words, "")
}

// pascal converts the given name into a PascalCase.
//
//	user_info 	=> UserInfo
//	full_name 	=> FullName
//	user_id   	=> UserID
//	full-admin	=> FullAdmin
func Pascal(s string) string {
	words := strings.FieldsFunc(s, isSeparator)
	return pascalWords(words)
}

func ruleset() *inflect.Ruleset {
	rules := inflect.NewDefaultRuleset()
	// Add common initialism from golint and more.
	for _, w := range []string{
		"ACL", "API", "ASCII", "AWS", "CPU", "CSS", "DNS", "EOF", "GB", "GUID",
		"HCL", "HTML", "HTTP", "HTTPS", "ID", "IP", "JSON", "KB", "LHS", "MAC",
		"MB", "QPS", "RAM", "RHS", "RPC", "SLA", "SMTP", "SQL", "SSH", "SSO",
		"TCP", "TLS", "TTL", "UDP", "UI", "UID", "URI", "URL", "UTF8", "UUID",
		"VM", "XML", "XMPP", "XSRF", "XSS",
	} {
		acronyms[w] = struct{}{}
		rules.AddAcronym(w)
	}
	return rules
}

func isSeparator(r rune) bool {
	return r == '_' || r == '-' || unicode.IsSpace(r)
}
