from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('users', '0031_add_nagging'),
    ]

    operations = [
        migrations.RemoveField(model_name='profile', name='shared_pdf_sorting'),
        migrations.RemoveField(model_name='profile', name='user_sorting'),
    ]
