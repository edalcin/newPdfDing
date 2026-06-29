from django.contrib.auth.models import User
from django.test import TestCase, override_settings
from users.models import Profile


class TestSignals(TestCase):
    @override_settings(DEFAULT_THEME='dark', DEFAULT_THEME_COLOR='Gray')
    def test_user_postsave(self):
        input_mail = 'a@a.com'

        user = User.objects.create_user(username='user', password='12345', email=input_mail)

        # check that the profile exists
        profile = Profile.objects.get(user=user)
        self.assertEqual(str(profile), input_mail)
        self.assertEqual(profile.current_workspace_id, str(user.id))
        self.assertEqual(profile.current_collection_id, str(user.id))
        self.assertEqual(profile.dark_mode, 'Dark')
        self.assertEqual(profile.theme_color, 'Gray')
