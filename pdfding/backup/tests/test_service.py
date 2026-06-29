import base64
from pathlib import Path
from unittest import mock

from backup import service
from django.test import TestCase


class TestEncryption(TestCase):
    mock_pbk_object = mock.Mock()
    mock_pbk_object.derive = lambda x: b'generated_key'

    mock_fernet_object = mock.Mock()
    mock_fernet_object.encrypt = lambda x: x + b'encrypted'
    mock_fernet_object.decrypt = lambda x: x + b'decrypted'

    def test_get_encryption_key_disabled(self):
        self.assertEqual(service.get_encryption_key(False, 'pw', 'salt'), None)

    @mock.patch('backup.service.generate_encryption_key', return_value=b'key')
    def test_get_encryption_key_enabled(self, mock_generate_encryption_key):
        self.assertEqual(service.get_encryption_key(True, 'pw', 'salt'), b'key')

    @mock.patch('backup.service.SHA256', return_value='sha256')
    @mock.patch('backup.service.PBKDF2HMAC', return_value=mock_pbk_object)
    def test_generate_encryption_key(self, mock_pbkdf2hmac, mock_sha256):
        generated_key = service.generate_encryption_key('pw', 'salt')

        mock_pbkdf2hmac.assert_called_with(
            algorithm='sha256',
            length=32,
            salt=b'salt',
            iterations=1000000,
        )

        self.assertEqual(generated_key, base64.urlsafe_b64encode(b'generated_key'))

    @mock.patch('backup.service.Fernet', return_value=mock_fernet_object)
    def test_encrypt_file(self, mock_fernet):
        parent_path = Path(__file__).parent
        tmp_file_path = parent_path / 'tmp'

        service.encrypt_file(b'key', parent_path / '__init__.py', tmp_file_path)

        with open(tmp_file_path, 'rb') as file:
            tmp_file_contents = file.read()

        # delete the tmp file
        tmp_file_path.unlink()

        self.assertEqual(tmp_file_contents, b'"""some content for encryption test"""\nencrypted')

    @mock.patch('backup.service.Fernet', return_value=mock_fernet_object)
    def test_decrypt_file(self, mock_fernet):
        parent_path = Path(__file__).parent
        tmp_file_path = parent_path / 'tmp'

        service.decrypt_file(b'key', parent_path / '__init__.py', tmp_file_path)

        with open(tmp_file_path, 'rb') as file:
            tmp_file_contents = file.read()

        # delete the tmp file
        tmp_file_path.unlink()

        self.assertEqual(tmp_file_contents, b'"""some content for encryption test"""\ndecrypted')
