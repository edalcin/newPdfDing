from os import environ

from .base import *  # noqa: F401 F403

try:
    from .version import VERSION  # pyright: ignore
except ModuleNotFoundError:
    VERSION = 'unknown'

# security related settings
# SECURITY WARNING: don't run with debug turned on in production!
DEBUG = False

STORAGES = {
    'default': {
        'BACKEND': 'django.core.files.storage.FileSystemStorage',
    },
    'staticfiles': {
        'BACKEND': 'whitenoise.storage.CompressedManifestStaticFilesStorage',
    },
}

# By default, Django’s hashed static files system creates two copies of each file in STATIC_ROOT:
# one using the original name, e.g. app.js, and one using the hashed name, e.g. app.db8f2edc0c8a.js.
# If WhiteNoise’s compression backend is being used this will create another two copies of each of
# these files (using Gzip and Brotli compression) resulting in six output files for each input file.
# In some deployment scenarios it can be important to reduce the size of the build artifact as much as possible.
# This setting removes the “un-hashed” version of the file (which should be not be referenced in any case)
# which should reduce the space required for static files by half.
WHITENOISE_KEEP_ONLY_HASHED_FILES = True

# web security settings
ALLOWED_HOSTS = environ.get('HOST_NAME', '').split(',')
ALLOWED_HOSTS = [allowed_host.strip() for allowed_host in ALLOWED_HOSTS if allowed_host != '']

if ALLOWED_HOSTS:
    CSRF_TRUSTED_ORIGINS = []
    for allowed_host in ALLOWED_HOSTS:
        CSRF_TRUSTED_ORIGINS.extend([f'https://{allowed_host}', f'http://{allowed_host}'])

SECRET_KEY = environ.get('SECRET_KEY')

if environ.get('CSRF_COOKIE_SECURE', 'TRUE') in ['TRUE', 'True']:
    CSRF_COOKIE_SECURE = True
if environ.get('SESSION_COOKIE_SECURE', 'TRUE') in ['TRUE', 'True']:
    SESSION_COOKIE_SECURE = True
if environ.get('SECURE_SSL_REDIRECT') in ['TRUE', 'True']:
    SECURE_SSL_REDIRECT = True
if environ.get('SECURE_HSTS_SECONDS'):
    SECURE_HSTS_SECONDS = environ.get('SECURE_HSTS_SECONDS')

# backup settings
if environ.get('BACKUP_ENABLE') in ['TRUE', 'True']:
    # without a dummy value, huey will not start
    BACKUP_ENABLED = True
    BACKUP_ENDPOINT = environ.get('BACKUP_ENDPOINT', 'minio.pdfding.com')
    BACKUP_ACCESS_KEY = environ.get('BACKUP_ACCESS_KEY')
    BACKUP_SECRET_KEY = environ.get('BACKUP_SECRET_KEY')
    BACKUP_REGION = environ.get('BACKUP_REGION', 'us-east-1')
    BACKUP_BUCKET_NAME = environ.get('BACKUP_BUCKET_NAME', 'pdfding')
    BACKUP_SCHEDULE = environ.get('BACKUP_SCHEDULE', '0 2 * * *')
    if environ.get('BACKUP_SECURE') in ['TRUE', 'True']:
        BACKUP_SECURE = True
    else:
        BACKUP_SECURE = False

    if environ.get('BACKUP_ENCRYPTION_ENABLE') in ['TRUE', 'True']:
        BACKUP_ENCRYPTION_ENABLED = True
        BACKUP_ENCRYPTION_PASSWORD = environ['BACKUP_ENCRYPTION_PASSWORD']
        BACKUP_ENCRYPTION_SALT = environ.get('BACKUP_ENCRYPTION_SALT', 'pdfding')
    else:
        BACKUP_ENCRYPTION_ENABLED = False
        # set to none, so that backups.tasks.backup_function raises no attribute error
        BACKUP_ENCRYPTION_PASSWORD = None
        BACKUP_ENCRYPTION_SALT = None
else:
    BACKUP_ENABLED = False
    BACKUP_SCHEDULE = '*/1 * * * *'

# consume settings
if environ.get('CONSUME_ENABLE') in ['TRUE', 'True']:
    CONSUME_ENABLED = True
    CONSUME_TAG_STRING = environ.get('CONSUME_TAGS', '')
    CONSUME_SCHEDULE = environ.get('CONSUME_SCHEDULE', '*/5 * * * *')
    if environ.get('CONSUME_SKIP_EXISTING') == 'FALSE':
        CONSUME_SKIP_EXISTING = False
    else:
        CONSUME_SKIP_EXISTING = True
else:
    CONSUME_ENABLED = False
    CONSUME_SKIP_EXISTING = False
    CONSUME_SCHEDULE = '*/5 * * * *'


# themes
theme_colors = ['green', 'blue', 'gray', 'red', 'pink', 'orange', 'brown']
themes = ['light', 'dark', 'system']

if not environ.get('DEFAULT_THEME'):
    DEFAULT_THEME = 'system'
elif environ.get('DEFAULT_THEME') in themes:
    DEFAULT_THEME = environ.get('DEFAULT_THEME')
else:
    raise ValueError(
        f'Provided DEFAULT_THEME value {environ.get('DEFAULT_THEME')} is not valid. '
        f'Valid values are: {", ".join(themes)}.'
    )

if not environ.get('DEFAULT_THEME_COLOR'):
    DEFAULT_THEME_COLOR = 'Green'
elif environ.get('DEFAULT_THEME_COLOR') in theme_colors:
    # tailwind css expects a leading capitalized letter, see pdfding/static/css/tailwind.css.
    DEFAULT_THEME_COLOR = environ.get('DEFAULT_THEME_COLOR', '').capitalize()
else:
    raise ValueError(
        f'Provided DEFAULT_THEME_COLOR value {environ.get('DEFAULT_THEME_COLOR')} is not valid. '
        f'Valid values are: {", ".join(theme_colors)}.'
    )

# Allow subdirectories when saving PDFs to the media dir in the UI
if environ.get('ALLOW_PDF_SUB_DIRECTORIES', 'FALSE') in ['TRUE', 'True']:
    ALLOW_PDF_SUB_DIRECTORIES = True
else:
    ALLOW_PDF_SUB_DIRECTORIES = False

