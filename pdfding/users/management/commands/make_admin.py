from django.contrib.auth.models import User
from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = "Give the user specified by the email address admin rights."

    def add_arguments(self, parser):
        parser.add_argument('-e', '--email', type=str, help='The email of the user who should be admin')

    def handle(self, *args, **kwargs):
        email = kwargs['email']
        user = User.objects.get(email=email)
        user.is_staff = True
        user.is_superuser = True
        user.save()
