from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('users', '0032_remove_dead_sorting_fields'),
    ]

    operations = [
        migrations.RemoveField(model_name='profile', name='last_time_nagged'),
    ]
