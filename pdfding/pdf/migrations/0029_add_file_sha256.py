import hashlib

from django.db import migrations, models


def backfill_sha256(apps, schema_editor):
    Pdf = apps.get_model('pdf', 'Pdf')
    for pdf in Pdf.objects.filter(file_sha256=''):
        try:
            with pdf.file.open('rb') as f:
                h = hashlib.sha256()
                for chunk in iter(lambda: f.read(8192), b''):
                    h.update(chunk)
                pdf.file_sha256 = h.hexdigest()
                pdf.save(update_fields=['file_sha256'])
        except Exception:
            pass  # missing file: leave blank


class Migration(migrations.Migration):

    dependencies = [
        ('pdf', '0028_remove_unneeded_fields_from_shared_collection'),
    ]

    operations = [
        migrations.AddField(
            model_name='pdf',
            name='file_sha256',
            field=models.CharField(blank=True, db_index=True, default='', max_length=64),
        ),
        migrations.RunPython(backfill_sha256, migrations.RunPython.noop),
    ]
