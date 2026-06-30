import logging

from django.core.management.base import BaseCommand

logger = logging.getLogger('management')


class Command(BaseCommand):
    help = "Clean up data on start up"

    def handle(self, *args, **kwargs):
        pass
