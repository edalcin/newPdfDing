from io import StringIO

from django.contrib.auth.models import User
from django.core.management import call_command
from django.test import TestCase
from pdf.models.pdf_models import Pdf
from pdf.services.pdf_services import PdfProcessingServices
from users.service import get_demo_pdf


class TestRegenerateThumbnails(TestCase):
    def setUp(self):
        self.user = User.objects.create_user(username='username', password='password', email='a@a.com')

    def test_regenerate_missing_thumbnail(self):
        # created directly via the model, bypassing PdfProcessingServices.create_pdf, so no thumbnail is set yet
        pdf = Pdf.objects.create(collection=self.user.profile.current_collection, name='pdf_1', file=get_demo_pdf())
        self.assertFalse(pdf.thumbnail)

        out = StringIO()
        call_command('regenerate_thumbnails', stdout=out)

        pdf.refresh_from_db()
        self.assertTrue(pdf.thumbnail)
        self.assertTrue(pdf.preview)
        self.assertIn('OK', out.getvalue())
        self.assertIn('1 succeeded, 0 failed', out.getvalue())

    def test_skips_pdfs_that_already_have_a_thumbnail(self):
        pdf = PdfProcessingServices.create_pdf(
            name='pdf_1', collection=self.user.profile.current_collection, pdf_file=get_demo_pdf()
        )
        old_thumbnail_name = pdf.thumbnail.name

        out = StringIO()
        call_command('regenerate_thumbnails', stdout=out)

        pdf.refresh_from_db()
        self.assertEqual(pdf.thumbnail.name, old_thumbnail_name)
        self.assertIn('No PDFs need thumbnail regeneration.', out.getvalue())

    def test_all_flag_regenerates_existing_thumbnails_too(self):
        pdf = PdfProcessingServices.create_pdf(
            name='pdf_1', collection=self.user.profile.current_collection, pdf_file=get_demo_pdf()
        )

        out = StringIO()
        call_command('regenerate_thumbnails', '--all', stdout=out)

        pdf.refresh_from_db()
        self.assertTrue(pdf.thumbnail)
        self.assertIn('1 succeeded, 0 failed', out.getvalue())
