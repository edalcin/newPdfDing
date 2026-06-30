import uuid

import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('pdf', '0030_remove_shared_models'),
    ]

    operations = [
        migrations.CreateModel(
            name='SharedPdf',
            fields=[
                ('id', models.UUIDField(default=uuid.uuid4, editable=False, primary_key=True, serialize=False)),
                ('creation_date', models.DateTimeField(auto_now_add=True)),
                ('views', models.IntegerField(default=0)),
                ('pdf', models.OneToOneField(on_delete=django.db.models.deletion.CASCADE, related_name='share', to='pdf.pdf')),
            ],
        ),
    ]
