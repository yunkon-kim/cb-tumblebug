#!/bin/bash

function CallSpider() {
    echo "- Get dataDisk in ${ResourceRegionNativeName}"

    resp=$(
        curl -H "${AUTH}" -sX POST http://$SpiderServer/spider/regdisk -H 'Content-Type: application/json' -d @- <<EOF
        { 
			"ConnectionName": "${CONN_CONFIG[$INDEX,$REGION]}",
			"ReqInfo": { 
				"Name": "${CONN_CONFIG[$INDEX,$REGION]}-${POSTFIX}", 
				"CSPId": "jhseo-test"
			}
		}
EOF
    ); echo ${resp} | jq '.'
    echo ""
}

#function spider_get_dataDisk() {

    echo "####################################################################"
	echo "## 11. dataDisk: Get"
	echo "####################################################################"

	source ../init.sh

	if [ "${INDEX}" == "0" ]; then
		echo "[Parallel execution for all CSP regions]"
		INDEXX=${NumCSP}
		for ((cspi = 1; cspi <= INDEXX; cspi++)); do
			INDEXY=${NumRegion[$cspi]}
			CSP=${CSPType[$cspi]}
			echo "[$cspi] $CSP details"
			for ((cspj = 1; cspj <= INDEXY; cspj++)); do
				echo "[$cspi,$cspj] ${RegionNativeName[$cspi,$cspj]}"

				ResourceRegionNativeName=${RegionNativeName[$cspi,$cspj]}

				INDEX=$cspi
				REGION=$cspj
				CallSpider
			done
		done
		wait

	else
		echo ""
		
		ResourceRegionNativeName=${CONN_CONFIG[$INDEX,$REGION]}

		CallSpider

	fi
    
#}

#spider_get_dataDisk
