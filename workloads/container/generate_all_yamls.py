# Write a python script to run go through each directory and generate the yaml files by running the python script inside each directory

import os
import subprocess

# Get the current working directory
yaml_dir = os.getcwd()+"/yamls"

# Get the list of directories
directories = [d for d in os.listdir(yaml_dir) if os.path.isdir(d)]

# Go through each directory and run the python script
for directory in directories:
    os.chdir(directory)
    # Run the python script
    subprocess.run(['python3', 'generate-yamls.py'])
    print(f"Generated yaml files for {directory}")
    os.chdir(current_dir)

print("Generated all yaml files")