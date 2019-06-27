#!/bin/bash

AGO=7d
while getopts "r:a:f:n:d" OPTION; do
  case $OPTION in
    r )
      REGISTRY=$OPTARG
      ;;
    a ) 
      AGO=$OPTARG
      ;;
    f ) 
      FILTER=$OPTARG
      ;;
    n ) 
      REPOSITORY=$OPTARG
      ;;
    d )
      DANGLING=1
      ;;
    ? ) 
        echo "ERR: Usage: acr.sh -r <REGISTRY> -n <REPOSITORY_NAME> -a <AGO> -f <FILTER>"
        exit 1
      ;;
  esac
done

if [ -z "$REPOSITORY" ]
   then
      echo ERR: -r Repository not set. 
      exit 1
fi

export ACR_REFRESH_TOKEN=$(cat ~/.docker/config.json | jq -r --arg r $REGISTRY '.auths[$r].identitytoken')
export SCOPE="repository:*:*"
export ACR_ACCESS_TOKEN=$(curl --silent -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "grant_type=refresh_token&service=$REGISTRY&scope=$SCOPE&refresh_token=$ACR_REFRESH_TOKEN" https://$REGISTRY/oauth2/token | jq -r '.access_token')

if [ -z "$ACR_ACCESS_TOKEN" ]
    then
        echo ERR: Unable to resolve authentication
        exit 1
fi

if [ -z "$FILTER" ]
  then
    if (($DANGLING))
      then
          acr purge -r $REGISTRY --ago $AGO --repository $REPOSITORY --access-token $ACR_ACCESS_TOKEN --dangling
      else
          acr purge -r $REGISTRY --ago $AGO --repository $REPOSITORY --access-token $ACR_ACCESS_TOKEN 
    fi
  else
    if (($DANGLING))
      then
          acr purge -r $REGISTRY --ago $AGO --filter $FILTER --repository $REPOSITORY --access-token $ACR_ACCESS_TOKEN --dangling
      else
          acr purge -r $REGISTRY --ago $AGO --filter $FILTER --repository $REPOSITORY --access-token $ACR_ACCESS_TOKEN 
    fi
fi
