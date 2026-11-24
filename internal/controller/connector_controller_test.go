/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
)

type codedErr struct{}

func (codedErr) Error() string          { return "tls: x509" }
func (codedErr) LoginErrorCode() string { return "tls" }

type formConnector struct{ plugin.Connector }

func (formConnector) ConnectorSlugName() string { return "ldap" }

// ib: form tabanlı girişte hata kodu login formuna taşınır; GET akışı upstream'deki gibi kalır.
func TestRedirectFormLoginError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if got := loginErrorCode(fmt.Errorf("sarmalı: %w", codedErr{})); got != "tls" {
		t.Fatalf("kod = %q", got)
	}
	if got := loginErrorCode(errors.New("kodsuz")); got != "unknown" {
		t.Fatalf("kod = %q", got)
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/answer/api/v1/connector/redirect/ldap", nil)
	if !redirectFormLoginError(ctx, formConnector{}, "tls") {
		t.Fatal("POST yönlendirilmeli")
	}
	if loc := w.Header().Get("Location"); ctx.Writer.Status() != http.StatusSeeOther || loc != "/users/login?error=ldap&reason=tls" {
		t.Fatalf("yanıt: %d %q", ctx.Writer.Status(), loc)
	}

	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/answer/api/v1/connector/redirect/basic", nil)
	if redirectFormLoginError(ctx, formConnector{}, "tls") {
		t.Fatal("GET akışına dokunulmamalı")
	}
}
