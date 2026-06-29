from .base import *  # noqa: F401 F403

try:
    from .dev_secrets import *  # noqa: F401 F403
except ModuleNotFoundError:  # pragma: no cover
    pass

# No social auth in single-admin mode

# Turn on debug mode
DEBUG = True
VERSION = 'DEV'

INTERNAL_IPS = ['127.0.0.1']

# SECURITY WARNING: keep the secret key used in production secret!
SECRET_KEY = 'some_key'  # nosec B105

DEFAULT_FROM_EMAIL = 'info@localhost'

# BACKUP
BACKUP_ENABLED = True
BACKUP_SECURE = False
BACKUP_ENDPOINT = '127.0.0.1:9000'
BACKUP_BUCKET_NAME = 'pdfding'
BACKUP_SCHEDULE = '*/1 * * * *'
BACKUP_ENCRYPTION_ENABLED = True
BACKUP_ENCRYPTION_SALT = 'pdfding'
BACKUP_REGION = 'us-east-1'

CONSUME_ENABLED = True
CONSUME_TAG_STRING = 'consumed file'
CONSUME_SCHEDULE = '*/1 * * * *'
CONSUME_SKIP_EXISTING = True

ALLOW_PDF_SUB_DIRECTORIES = True

# check if minio access and secret keys are set in dev_secrets
if 'BACKUP_ACCESS_KEY' not in locals():
    BACKUP_ACCESS_KEY = 'add_access_key'
if 'BACKUP_SECRET_KEY' not in locals():
    BACKUP_SECRET_KEY = 'add_secret_key'  # nosec

# check if backup encryption password is set in dev_secrets
if 'BACKUP_ENCRYPTION_PASSWORD' not in locals():
    BACKUP_ENCRYPTION_PASSWORD = 'password'  # nosec

# themes
DEFAULT_THEME = 'dark'
DEFAULT_THEME_COLOR = 'Green'

ADMIN_EMAIL = 'admin@example.com'
ADMIN_PASSWORD = 'changeme'
