#!/usr/bin/env python3

import os
import base64
import time
import sys
import argparse
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
import subprocess
import requests
import yaml
from tqdm import tqdm
from colorama import Fore, init
from getpass import getpass

parser = argparse.ArgumentParser(description="Automatically proceed without confirmation.")
parser.add_argument('-y', '--yes', action='store_true', help='Automatically answer yes to prompts and proceed.')
args = parser.parse_args()

# Initialize colorama
init(autoreset=True)

# Configuration
TUMBLEBUG_SERVER = os.getenv('TUMBLEBUG_SERVER', 'localhost:1323')
TB_API_USERNAME = os.getenv('TB_API_USERNAME', 'default')
TB_API_PASSWORD = os.getenv('TB_API_PASSWORD', 'default')
AUTH = f"Basic {base64.b64encode(f'{TB_API_USERNAME}:{TB_API_PASSWORD}'.encode()).decode()}"
HEADERS = {'Authorization': AUTH, 'Content-Type': 'application/json'}

CRED_FILE_NAME_ENC = "credentials.yaml.enc"
CRED_PATH = os.path.join(os.path.expanduser('~'), '.cloud-barista')
ENC_FILE_PATH = os.path.join(CRED_PATH, CRED_FILE_NAME_ENC)
KEY_FILE = os.path.join(CRED_PATH, ".tmp_enc_key")

expected_completion_time_seconds = 240

# Check for credential path
if not os.path.exists(CRED_PATH):
    print(Fore.RED + "Error: CRED_PATH does not exist. Please run scripts/genCredential.sh first.")
    sys.exit(1)
elif not os.path.isfile(ENC_FILE_PATH):
    print(Fore.RED + f"Error: {CRED_FILE_NAME_ENC} does not exist. Please check if it has been generated.")
    print(Fore.RED + f"- This script does not accept 'credentials.yaml'. For your security, it only accepts an encrypted file.")
    print(Fore.RED + f"- Please generate '{CRED_FILE_NAME_ENC}' using 'init/encCredential.sh'.")
    sys.exit(1)


# Decrypt credentials.yaml.enc
def decrypt_credentials(enc_file_path, key):
    try:
        result = subprocess.run(
            ['openssl', 'enc', '-aes-256-cbc', '-d', '-pbkdf2', '-in', enc_file_path, '-pass', f'pass:{key}'],
            check=True,
            capture_output=True
        )
        if result.returncode != 0:
            return None, "Decryption failed."
        return result.stdout.decode('utf-8'), None
    except subprocess.CalledProcessError as e:
        return None, f"Decryption error: {e.stderr.decode('utf-8')}"

def get_decryption_key():
    # Try using the key from the key file
    if os.path.isfile(KEY_FILE):
        with open(KEY_FILE, 'r') as kf:
            key = kf.read().strip()
            print(Fore.YELLOW + f"Using key from {KEY_FILE} to decrypt the credentials file.")
            decrypted_content, error = decrypt_credentials(ENC_FILE_PATH, key)
            if error is None:
                return decrypted_content
            print(Fore.RED + error)

    # Prompt for password up to 3 times if the key file is not used or fails
    for attempt in range(3):
        password = getpass(f"Enter the password to decrypt the credentials file (attempt {attempt + 1}/3): ")
        decrypted_content, error = decrypt_credentials(ENC_FILE_PATH, password)
        if error is None:
            return decrypted_content
        print(Fore.RED + error)

    print(Fore.RED + "Failed to decrypt the file after 3 attempts. Exiting.")
    sys.exit(1)

# Print the current configuration
print(Fore.YELLOW + "Current Configuration\nPlease set the corresponding environment variables to make changes.")
print(" - " + Fore.CYAN + "TUMBLEBUG_SERVER:" + Fore.RESET + f" {TUMBLEBUG_SERVER}")
print(" - " + Fore.CYAN + "TB_API_USERNAME:" + Fore.RESET + f" {TB_API_USERNAME[0]}**********")
print(" - " + Fore.CYAN + "TB_API_PASSWORD:" + Fore.RESET + f" {TB_API_PASSWORD[0]}**********")
print(" - " + Fore.CYAN + "CRED_PATH:" + Fore.RESET + f" {CRED_PATH}")
print(" - " + Fore.CYAN + "CRED_FILE_NAME:" + Fore.RESET + f" {CRED_FILE_NAME_ENC}")
print(" - " + Fore.CYAN + "expected completion time:" + Fore.RESET + f" {expected_completion_time_seconds} seconds\n")

# Check server health before proceeding
print(Fore.YELLOW + "Checking server health...")
health_check_url = f"http://{TUMBLEBUG_SERVER}/tumblebug/readyz"
try:
    health_response = requests.get(health_check_url, headers=HEADERS)
    if health_response.status_code == 200:
        print(Fore.GREEN + "Tumblebug Server is healthy.\n")
    else:
        print(Fore.RED + f"Tumblebug health check failed with status {health_response.status_code}.")
        sys.exit(1)
except requests.exceptions.RequestException as e:
    print(Fore.RED + f"Failed to connect to server. Check the server address and try again.")
    sys.exit(1)

# Wait for user input to proceed
print(Fore.YELLOW + "Registering credentials and Loading common Specs and Images takes time")
if not args.yes:
    if input(Fore.CYAN + 'Do you want to proceed ? (y/n) : ').lower() not in ['y', 'yes']:
        print(Fore.GREEN + "Cancel [{}]".format(' '.join(sys.argv)))
        print(Fore.GREEN + "See you soon. :)")
        sys.exit(0)

# Get the decryption key and decrypt the credentials file
decrypted_content = get_decryption_key()
cred_data = yaml.safe_load(decrypted_content)['credentialholder']['admin']

print(Fore.YELLOW + f"\nRegistering all valid credentials for all cloud regions...")

# Function to register credentials
def register_credential(provider, credentials):
    try:
        if all(credentials.values()):
            credential_payload = {
                "credentialHolder": "admin",
                "keyValueInfoList": [{"key": k, "value": v} for k, v in credentials.items()],
                "providerName": provider
            }
            response = requests.post(f"http://{TUMBLEBUG_SERVER}/tumblebug/credential", json=credential_payload, headers=HEADERS)
            return provider, response.json(), Fore.GREEN
        else:
            return provider, "Incomplete credential data, Skip", Fore.RED
    except Exception as e:
        return provider, f"Error registering credentials: {str(e)}", Fore.RED

# Register credentials to TumblebugServer using ThreadPoolExecutor
with ThreadPoolExecutor(max_workers=5) as executor:
    future_to_provider = {executor.submit(register_credential, provider, credentials): provider for provider, credentials in cred_data.items()}
    for future in as_completed(future_to_provider):
        provider, response, color = future.result()
        print(color + f"- {provider}: {response}")

print(Fore.YELLOW + "\nLoading common Specs and Images...")
print(Fore.RESET)

# Function to perform the HTTP request and handle exceptions
def load_resources():
    global response_json
    try:
        response = requests.get(f"http://{TUMBLEBUG_SERVER}/tumblebug/loadCommonResource", headers=HEADERS)
        response.raise_for_status()  # Will raise an exception for HTTP error codes
        response_json = response.json()
        if response_json is None:  # Check if response.json() returned None
            response_json = {'error': 'No content returned'}
        if 'output' not in response_json:
            response_json = {'error': 'No output content returned'}
        if response_json.get('output', []) is None:
            response_json = {'error': 'Empty output content returned'}
    except requests.RequestException as e:
        response_json = {'error': str(e)}
    finally:
        event.set()  # Signal that the request is complete regardless of success or failure

# Start time
start_time = time.time()

# Event object to signal the request completion
event = threading.Event()

# Start the network request in a separate thread
thread = threading.Thread(target=load_resources)
thread.start()

# Expected duration and progress bar
with tqdm(total=expected_completion_time_seconds, desc="Progress", unit='s') as pbar:
    while not event.is_set():
        time.sleep(1)
        pbar.update(1)
    pbar.update(expected_completion_time_seconds - pbar.n)  # Ensure the progress bar completes

# Wait for the thread to complete
thread.join()

# Calculate duration
end_time = time.time()
duration = end_time - start_time
minutes = duration / 60

# Handling output based on the API response
if 'error' in response_json:
    print(Fore.RED + "Error during resource loading: " + response_json['error'])
    exit(1)
elif response_json: 
    failed_specs = 0
    failed_images = 0
    successful_specs = 0
    successful_images = 0

    for item in response_json.get('output', []):  
        if "spec:" in item:
            if "[Failed]" in item:
                failed_specs += 1
            else:
                successful_specs += 1
        elif "image:" in item:
            if "[Failed]" in item:
                failed_images += 1
            else:
                successful_images += 1

    print(Fore.CYAN + f"\nLoading completed ({minutes:.2f} minutes)")
    print(Fore.RESET + "Registered Common specs")
    print(Fore.GREEN + f"- Successful: {successful_specs}" + Fore.RESET + f", Failed: {failed_specs}")
    print(Fore.RESET + "Registered Common images")
    print(Fore.GREEN + f"- Successful: {successful_images}" + Fore.RESET + f", Failed: {failed_images}")
else:
    print(Fore.RED + "No data returned from the API.")
