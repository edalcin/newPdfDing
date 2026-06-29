from django.contrib.auth.models import User
from django.test import TestCase
from django.urls import reverse


class TestAdminViews(TestCase):
    def setUp(self):
        self.user = User.objects.create_user(username='admin', password='password', email='a@a.com')
        self.user.is_staff = True
        self.user.is_superuser = True
        self.user.save()

        self.client.login(username='admin', password='password')

    def test_get_information(self):
        response = self.client.get(reverse('instance_info'))

        self.assertEqual(response.status_code, 200)
