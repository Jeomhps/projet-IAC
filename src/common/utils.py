import subprocess

def sha512_hash_openssl(password: str) -> str:
    result = subprocess.run(['openssl', 'passwd', '-6', password], capture_output=True, text=True)
    return result.stdout.strip()
