// Copyright 2022 Datafuse Labs.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package golake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

var (
	ProvisionWarehouseTimeout = "ProvisionWarehouseTimeout"

	ErrDoRequest    = errors.New("DoRequestFailed")
	ErrReadResponse = errors.New("ReadResponseFailed")
)

type APIErrorResponseBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type APIError struct {
	RespBody   APIErrorResponseBody
	RespText   string
	StatusCode int
	Hint       string
}

func (e APIError) Error() string {
	// Upstream messages and response bodies may echo SQL, credentials or proxy
	// diagnostics. Keep them available through the explicit fields/accessor, but
	// never include them in the default diagnostic representation.
	message := fmt.Sprintf("HTTP %d", e.StatusCode)
	if e.Hint != "" {
		message += ": " + e.Hint
	}
	return message
}

// Format keeps debug formats such as %#v from dumping the raw response fields.
func (e APIError) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		_, _ = fmt.Fprintf(state, "%q", e.Error())
		return
	}
	_, _ = io.WriteString(state, e.Error())
}

// LogValue provides safe metadata to structured loggers without serializing the response.
func (e APIError) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("status_code", e.StatusCode), slog.String("hint", e.Hint))
}

// isProvisionWarehouseTimeout checks the response explicitly: Error() intentionally
// omits it, but changing diagnostic output must not change the retry decision.
func isProvisionWarehouseTimeout(err error) bool {
	var apiErr APIError
	if errors.As(err, &apiErr) {
		message := apiErr.RespBody.Message
		if message == "" {
			message = apiErr.RespText
		}
		return strings.Contains(message, ProvisionWarehouseTimeout) ||
			strings.Contains(apiErr.Hint, ProvisionWarehouseTimeout)
	}
	return strings.Contains(err.Error(), ProvisionWarehouseTimeout)
}

func NewAPIError(hint string, status int, respBuf []byte) error {
	respBody := APIErrorResponseBody{}
	_ = json.Unmarshal(respBuf, &respBody)
	return APIError{
		RespBody:   respBody,
		RespText:   string(respBuf),
		StatusCode: status,
		Hint:       hint,
	}
}

func IsNotFound(err error) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

func IsProxyErr(err error) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 520
}

func IsAuthFailed(err error) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 401
}

func isBadGateway(err error) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 502
}

func RespBody(err error) APIErrorResponseBody {
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		return APIErrorResponseBody{}
	}
	return apiErr.RespBody
}
