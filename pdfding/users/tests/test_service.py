
import users.service as service
from django.test import TestCase, override_settings


class TestUserServices(TestCase):
    def test_clean_convert_hex_to_rgb_correct(self):
        self.assertEqual(service.convert_hex_to_rgb('#FFB3A5'), (255, 179, 165))
        self.assertEqual(service.convert_hex_to_rgb('ffb3a5'), (255, 179, 165))

    def test_convert_rgb_to_hex(self):
        self.assertEqual(service.convert_rgb_to_hex(255, 179, 165), '#ffb3a5')

    def test_get_color_shades(self):
        self.assertEqual(service.get_secondary_color('#b5edff'), '#91becc')

    def test_get_viewer_colors_profile(self):
        user = User.objects.create_user(username='user', password="password")

        profile = user.profile
        profile.theme_color = 'Green'
        profile.dark_mode = 'light'

        generated_theme, generated_color = service.get_viewer_theme_and_color(profile)

        self.assertEqual(generated_color, '74 222 128')
        self.assertEqual(generated_theme, 'light')

        # also test custom color and inverted mode
        profile.theme_color = 'Custom'
        profile.pdf_inverted_mode = 'Enabled'
        profile.custom_theme_color = '#000000'

        generated_theme, generated_color = service.get_viewer_theme_and_color(profile)

        self.assertEqual(generated_color, '0 0 0')
        self.assertEqual(generated_theme, 'inverted')

    @override_settings(DEFAULT_THEME='dark', DEFAULT_THEME_COLOR='Brown')
    def test_get_viewer_colors_no_profile(self):
        generated_theme, generated_color = service.get_viewer_theme_and_color()

        self.assertEqual(generated_color, '76 37 24')
        self.assertEqual(generated_theme, 'dark')

