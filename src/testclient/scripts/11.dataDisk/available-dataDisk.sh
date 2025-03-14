#!/bin/bash

echo "####################################################################"
echo "## 11. dataDisk: Get available dataDisks"
echo "####################################################################"

source ../init.sh

curl -H "${AUTH}" -sX GET http://$TumblebugServer/tumblebug/ns/$NSID/mci/${MCIID}/vm/${CONN_CONFIG[$INDEX,$REGION]}-1/dataDisk | jq '.'
