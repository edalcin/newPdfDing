from django.forms import ValidationError
from django.test import TestCase
from users.forms import clean_hex_color


class TestUserForms(TestCase):
    def test_clean_custom_theme_color_correct(self):
        self.assertEqual(clean_hex_color('#FFB3A5'), '#ffb3a5')

    def test_clean_custom_theme_color_incorrect(self):
        with self.assertRaisesMessage(
            ValidationError, expected_message='Only valid hex colors are allowed! E.g.: #ffa385.'
        ):
            clean_hex_color('FFB3A5')

        with self.assertRaisesMessage(
            ValidationError, expected_message='Only valid hex colors are allowed! E.g.: #ffa385.'
        ):
            clean_hex_color('#FFF')
