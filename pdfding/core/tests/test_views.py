from django.test import TestCase
from django.urls import reverse


class TestLoginRequired(TestCase):
    def test_normal_mode(self):
        response = self.client.get(reverse('healthz'))

        self.assertEqual(response.status_code, 200)
