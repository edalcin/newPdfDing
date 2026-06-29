import os
import sys

from django.contrib.auth.models import User
from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = 'Create or update the single admin user from env vars ADMIN_EMAIL and ADMIN_PASSWORD.'

    def handle(self, *args, **kwargs):
        email = os.environ.get('ADMIN_EMAIL', '')
        password = os.environ.get('ADMIN_PASSWORD', '')

        if not email or not password:
            self.stderr.write('ADMIN_EMAIL and ADMIN_PASSWORD must be set.')
            sys.exit(1)

        user, created = User.objects.get_or_create(
            email=email,
            defaults={'username': email},
        )
        user.is_staff = True
        user.is_superuser = True
        user.set_password(password)
        user.save()

        if created:
            self.stdout.write(f'Admin user created: {email}')
        else:
            self.stdout.write(f'Admin user updated: {email}')
