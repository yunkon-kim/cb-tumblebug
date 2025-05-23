#!/bin/bash

echo "####################################################################"
echo "## 7. spec: filter"
echo "####################################################################"

source ../init.sh

resp=$(
    curl -H "${AUTH}" -sX POST http://$TumblebugServer/tumblebug/ns/$NSID/resources/filterSpecs -H 'Content-Type: application/json' -d @- <<EOF
	    { 
		    "vCPU": 1, 
		    "memoryGiB": 2
	    }
EOF
)
echo ${resp} | jq '.'
echo ""
