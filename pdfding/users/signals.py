from django.conf import settings
from django.contrib.auth.models import User
from django.db.models.signals import post_save
from django.dispatch import receiver

from .models import Profile


@receiver(post_save, sender=User)
def user_postsave(sender, instance, created, **kwargs):
    """Create the corresponding profile if a user is created."""

    if created:
        profile = Profile.objects.create(user=instance)
        profile.dark_mode = Profile.DarkMode[str.upper(settings.DEFAULT_THEME)]
        profile.theme_color = Profile.ThemeColor[str.upper(settings.DEFAULT_THEME_COLOR)]
        profile.current_workspace_id = str(instance.id)
        profile.current_collection_id = str(instance.id)
        profile.save()
