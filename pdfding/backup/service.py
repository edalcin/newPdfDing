import base64
from pathlib import Path

from cryptography.fernet import Fernet
from cryptography.hazmat.primitives.hashes import SHA256
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC


def get_encryption_key(encryption_enabled: bool, password: str, salt: str):
    """
    Get the encryption key if encryption is activated, otherwise return None.
    """

    if encryption_enabled:
        return generate_encryption_key(password, salt)
    else:
        return None


def generate_encryption_key(password: str, salt: str) -> bytes:
    """
    Generate the encryption key with PBKDF2 using a user provided password and salt.
    """

    password_b = bytes(password, encoding='utf-8')
    salt_b = bytes(salt, encoding='utf-8')

    kdf = PBKDF2HMAC(
        algorithm=SHA256(),
        length=32,
        salt=salt_b,
        iterations=1000000,
    )

    encryption_key = base64.urlsafe_b64encode(kdf.derive(password_b))

    return encryption_key


def encrypt_file(encryption_key: bytes, source_path: Path, target_path: Path):
    """
    Encrypt the specified file using Fernet. Fernet is a symmetric encryption algorithm provided by the cryptography
    library. The encryption key should be generated using PBKDF2.
    """

    with open(source_path, 'rb') as file:
        unencrypted = file.read()

    f = Fernet(encryption_key)
    encrypted = f.encrypt(unencrypted)

    with open(target_path, 'wb') as encrypted_file:
        encrypted_file.write(encrypted)


def decrypt_file(encryption_key: bytes, source_path: Path, target_path: Path):
    """
    Decrypt the specified file using Fernet. Fernet is a symmetric encryption algorithm provided by the cryptography
    library. The encryption key should be generated using PBKDF2.
    """

    f = Fernet(encryption_key)

    # opening the encrypted file
    with open(source_path, 'rb') as enc_file:
        encrypted = enc_file.read()

    # decrypting the file
    decrypted = f.decrypt(encrypted)

    # create the directory if missing
    target_path.parent.mkdir(exist_ok=True, parents=True)

    # opening the file in write mode and
    # writing the decrypted data
    with open(target_path, 'wb') as dec_file:
        dec_file.write(decrypted)
