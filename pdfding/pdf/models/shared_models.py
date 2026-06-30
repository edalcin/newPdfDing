from uuid import uuid4

from django.db import models
from pdf.models.pdf_models import Pdf


class SharedPdf(models.Model):
    """A public, read-only share link for a single PDF."""

    id = models.UUIDField(primary_key=True, default=uuid4, editable=False)
    pdf = models.OneToOneField(Pdf, on_delete=models.CASCADE, related_name='share')
    creation_date = models.DateTimeField(auto_now_add=True)
    views = models.IntegerField(default=0)

    def __str__(self) -> str:  # pragma: no cover
        return f'Share of {self.pdf.name}'
