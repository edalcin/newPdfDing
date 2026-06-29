from django.apps import AppConfig


class PdfConfig(AppConfig):
    default_auto_field = 'django.db.models.BigAutoField'
    name = 'pdf'

    def ready(self):
        import pdf.signals  # noqa: F401
