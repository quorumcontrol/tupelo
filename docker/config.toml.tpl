NotaryGroupConfig = "${TUPELO_NOTARY_GROUP_CONFIG}"

BootstrapOnly = ${TUPELO_BOOTSTRAP_ONLY}
Namespace = "${TUPELO_NAMESPACE}"
Port = ${TUPELO_PORT}
WebSocketPort = ${TUPELO_WEB_SOCKET_PORT}
SecureWebSocketDomain = "${TUPELO_SECURE_WEB_SOCKET_DOMAIN}"
CertificateCache = "${TUPELO_CERTIFICATE_CACHE}"
PublicIP = "${TUPELO_PUBLIC_IP}"
TracingSystem = "${TUPELO_TRACING_SYSTEM}"

[PrivateKeySet]
SignKeyHex = "${TUPELO_SIGN_KEY_HEX}"
DestKeyHex = "${TUPELO_DEST_KEY_HEX}"

[Storage]
Kind = "${TUPELO_STORAGE_KIND}"
# used when Kind = "badger"
Path = "${TUPELO_STORAGE_PATH}"
# used when Kind = "s3"
Bucket = "${TUPELO_STORAGE_S3_BUCKET}"
Region = "${TUPELO_STORAGE_S3_REGION}"
RootDirectory = "${TUPELO_STORAGE_S3_PATH_PREFIX}"
