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

package plugin

import "context"

// ib: Bu dosya fork yamasıdır (upstream'e önerilecek). Importer yalnız soru
// açabiliyor ve ID döndürmüyor; hook'larla gelen içeriğin sonradan
// güncellenip kapatılabilmesi için ContentWriter eklendi. Mevcut dosyalara
// dokunulmaz: eklenti init() içinde plugin.Register'a ek olarak
// RegisterContentWriter çağırır.

// QuestionWriteInfo bir sorunun yazılacak hali. Yazar UserEmail ile bulunur.
type QuestionWriteInfo struct {
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
	UserEmail   string   `json:"user_email"`
	EditSummary string   `json:"edit_summary"`
}

// ContentFunc core'un eklentilere açtığı yazma işlemleri. ctx bir HTTP
// isteğinin *gin.Context'i olmalıdır (eklenti rotasından çağrılır).
type ContentFunc interface {
	// CreateQuestion soruyu açar ve kısa ID'sini döndürür.
	CreateQuestion(ctx context.Context, info QuestionWriteInfo) (questionID string, err error)
	// UpdateQuestion soruyu günceller; yalnız sorunun sahibi (aynı e-posta) güncelleyebilir.
	UpdateQuestion(ctx context.Context, questionID string, info QuestionWriteInfo) (err error)
	// CloseQuestion soruyu kapatır; yalnız sorunun sahibi kapatabilir.
	CloseQuestion(ctx context.Context, questionID, userEmail, message string) (err error)
	// ReopenQuestion kapalı soruyu yeniden açar; yalnız sorunun sahibi.
	ReopenQuestion(ctx context.Context, questionID, userEmail string) (err error)
}

type ContentWriter interface {
	Base
	RegisterContentFunc(ctx context.Context, fn ContentFunc)
}

var (
	// CallContentWriter kayıtlı bütün ContentWriter eklentilerini çağırır.
	CallContentWriter,
	registerContentWriter = MakePlugin[ContentWriter](false)
)

// RegisterContentWriter eklentinin init() fonksiyonundan çağrılır.
func RegisterContentWriter(p ContentWriter) { registerContentWriter(p) }
