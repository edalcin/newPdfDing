from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest.mock import patch

from django.contrib.auth.models import User
from django.core.management import call_command
from django.test import TestCase
from pdf.models.pdf_models import Pdf
from pdf.models.shared_models import SharedPdf
from users.management.commands.clean_up import clean_up_deleted_shared_pdfs


class TestManagement(TestCase):
    def test_make_admin(self):
        User.objects.create_user(username='user', password='12345', email='a@a.com')
        call_command('make_admin', email='a@a.com')
        user = User.objects.get(email='a@a.com')

        self.assertTrue(user.is_staff)
        self.assertTrue(user.is_superuser)

    @patch('users.management.commands.clean_up.clean_up_deleted_shared_pdfs')
    def test_clean_up_normal_mode(self, mock_clean_up_deleted_shared_pdfs):
        call_command('clean_up')

        mock_clean_up_deleted_shared_pdfs.assert_called_once_with()

    def test_clean_up_shared_pdfs(self):
        user = User.objects.create_user(username='user', password='password', email='a@a.com')
        pdf = Pdf.objects.create(name='1.pdf', collection=user.profile.current_collection)

        shared_pdf_1 = SharedPdf.objects.create(
            name='shared_pdf_1',
            pdf=pdf,
            deletion_date=datetime.now(timezone.utc) - timedelta(minutes=5),
        )
        shared_pdf_2 = SharedPdf.objects.create(
            name='shared_pdf_2',
            pdf=pdf,
            deletion_date=datetime.now(timezone.utc) + timedelta(minutes=5),
        )
        shared_pdf_3 = SharedPdf.objects.create(name='shared_pdf_3', pdf=pdf)

        shared_pdfs_set_before = {shared_pdf for shared_pdf in SharedPdf.objects.all()}
        self.assertEqual(shared_pdfs_set_before, {shared_pdf_1, shared_pdf_2, shared_pdf_3})

        clean_up_deleted_shared_pdfs()

        shared_pdfs_set_after = {shared_pdf for shared_pdf in SharedPdf.objects.all()}
        self.assertEqual(shared_pdfs_set_after, {shared_pdf_2, shared_pdf_3})
