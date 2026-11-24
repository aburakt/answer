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

package i18n

const (
	InfoName        = "plugin.ldap_connector.backend.info.name"
	InfoDescription = "plugin.ldap_connector.backend.info.description"
	ConnectorName   = "plugin.ldap_connector.backend.name"

	configPrefix = "plugin.ldap_connector.backend.config."
	TestOK       = "plugin.ldap_connector.backend.test.ok"
	TestUserOK   = "plugin.ldap_connector.backend.test.user_ok"
	TestFailed   = "plugin.ldap_connector.backend.test.failed"
	SecurityTLS  = "plugin.ldap_connector.backend.security.tls"
	SecurityNone = "plugin.ldap_connector.backend.security.none"
	ReasonPrefix = "plugin.ldap_connector.backend.reason."
)

// ConfigTitle ve ConfigDescription bir ayar alanının çeviri anahtarlarını verir.
func ConfigTitle(field string) string       { return configPrefix + field + ".title" }
func ConfigDescription(field string) string { return configPrefix + field + ".description" }
func ConfigLabel(field string) string       { return configPrefix + field + ".label" }
