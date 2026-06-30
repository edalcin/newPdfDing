from django.db import migrations


class Migration(migrations.Migration):

    dependencies = [
        ('pdf', '0029_add_file_sha256'),
        ('sessions', '0001_initial'),
    ]

    operations = [
        migrations.DeleteModel(name='SharedCollection'),
        migrations.DeleteModel(name='SharedPdf'),
    ]
