from django.core.management.base import BaseCommand
from django.db.models import Q
from pdf.models.pdf_models import Pdf
from pdf.services.pdf_services import PdfProcessingServices


class Command(BaseCommand):
    help = (
        'Regenerate thumbnails/previews for PDFs that are missing one, e.g. because generation failed on upload. '
        'Run with LOG_LEVEL=INFO or check the logs afterwards to see the reason for any remaining failures.'
    )

    def add_arguments(self, parser):
        parser.add_argument(
            '--all',
            action='store_true',
            help='Regenerate for every PDF instead of only the ones missing a thumbnail or preview.',
        )

    def handle(self, *args, **kwargs):
        if kwargs['all']:
            pdfs = Pdf.objects.all()
        else:
            missing = Q(thumbnail='') | Q(thumbnail__isnull=True) | Q(preview='') | Q(preview__isnull=True)
            pdfs = Pdf.objects.filter(missing)

        if not pdfs:
            self.stdout.write('No PDFs need thumbnail regeneration.')
            return

        failures = 0

        for pdf in pdfs:
            PdfProcessingServices.process_with_pypdfium(pdf, delete_existing_thumbnail_and_preview=bool(pdf.thumbnail))

            if pdf.thumbnail:
                self.stdout.write(f'OK     "{pdf.name}" ({pdf.id})')
            else:
                failures += 1
                self.stderr.write(f'FAILED "{pdf.name}" ({pdf.id}) - see logs for the traceback')

        self.stdout.write(f'Done. {pdfs.count() - failures} succeeded, {failures} failed.')
