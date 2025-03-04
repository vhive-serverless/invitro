# Write a script to generate the deploy info for each yaml file based on the directory and write to a json calleds deploy_info.json

# Example
# "aes-python-20000-20200": {
#        "yaml-location": "./yamls/aes-python/kn-aes-python-20000-20200.yaml",
#        "predeployment-commands": []
#    },
#
import os
import glob
import json

# Get the current working directory
current_dir = os.getcwd()+"/yamls"
print(current_dir)
# Get the list of directories
directories = [d for d in os.listdir(current_dir) if d != "deploy_info.json"]
print(directories)
predeployment_required = ["hotel-app", "image-rotate-python", "image-rotate-go", "video-processing", "video-analytics-standalone", "online-shop"]

deploy_info = {}
for directory in directories:
    os.chdir(os.path.join(current_dir, directory))
    # Get the list of yaml files
    yaml_files = glob.glob("*.yaml")
    function_name = {}
    for yaml_file in yaml_files:
        function_name[yaml_file] = yaml_file.replace(".yaml", "")
        function_name[yaml_file] = function_name[yaml_file].replace("kn-", "")
    for yaml_file in yaml_files:
        if directory in predeployment_required:
            if directory == "hotel-app":
                deploy_info[directory + '-' + function_name[yaml_file]] = {
                    "YamlLocation": f"workloads/container/yamls/{directory}/{yaml_file}",
                    "PredeploymentPath": [f"workloads/container/yamls/hotel-app/database.yaml"]
                }
            elif directory == "online-shop":
                deploy_info[function_name[yaml_file]] = {
                    "YamlLocation": f"workloads/container/yamls/{directory}/{yaml_file}",
                    "PredeploymentPath": [f"workloads/container/yamls/online-shop/database.yaml"]
                }
            elif directory == "image-rotate-go" or directory == "image-rotate-python":
                deploy_info[function_name[yaml_file]] = {
                    "YamlLocation": f"workloads/container/yamls/{directory}/{yaml_file}",
                    "PredeploymentPath": [f"workloads/container/yamls/{directory}/image-rotate-database.yaml"]
                }
            elif directory == "video-processing":
                deploy_info[function_name[yaml_file]] = {
                    "YamlLocation": f"workloads/container/yamls/{directory}/{yaml_file}",
                    "PredeploymentPath": [f"workloads/container/yamls/{directory}/video-processing-database.yaml"]
                }
            elif directory == "video-analytics-standalone":
                deploy_info[function_name[yaml_file]] = {
                    "YamlLocation": f"workloads/container/yamls/{directory}/{yaml_file}",
                    "PredeploymentPath": [f"workloads/container/yamls/{directory}/video-analytics-standalone-database.yaml"]
                }
        else:
            deploy_info[function_name[yaml_file]] = {
                "YamlLocation": f"workloads/container/yamls/{directory}/{yaml_file}",
                "PredeploymentPath": []
            }


    os.chdir(current_dir)

# Write the deploy info to a json file
deploy_keys = sorted(deploy_info)
sorted_deploy_info = {}
for key in deploy_keys:
    sorted_deploy_info[key] = deploy_info[key]
with open("deploy_info.json", "w") as f:
    f.write(json.dumps(sorted_deploy_info, indent=4))
print("Generated deploy info json file")

