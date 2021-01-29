#!/bin/zsh

PrintHelp() {
  printf "\nUsage: %s --i2Host localhost:5665 --i2Auth root:icinga --iw2Host 192.168.178.50:80 --iw2Auth icingaadmin:icinga --cfdPath /etc/icinga2/conf.d" $0

  printf "\n\t--i2Host Icinga 2 Host localhost:5665\n"
  printf "\t--i2Auth Icinga 2 API authentication root:icinga\n"
  printf "\t--iw2Host Icinga Web 2 url\n"
  printf "\t--iw2Auth Icing Web 2 authentication username:password\n"
  printf "\t--cfdPath Absolute path to icinga2 conf.d directory\n"
  exit 1
}

while echo $1 | grep -q ^-; do
  declare "$(echo $1 | sed 's/^-//')=$2"
  shift 2
done

if [ -z "$i2Host" ] || [ -z "$i2Auth" ] || [ -z "$iw2Host" ] || [ -z "$iw2Auth" ] || [ -z "$cfdPath" ]; then
  printf "\nSome of the required parameters are empty!\n"
  PrintHelp
fi

CreateHost() {
  curl=("curl" -skSu "$i2Auth" -H 'Accept: application/json' -X PUT "$@")
  resp=$("${curl[@]}")
  if [[ $resp == "[]" ]]; then
    return 1
  else
    return 0
  fi
}

GetHost() {
  curl=("curl" -skSu "$1" -H 'Accept: application/json' -X GET "$2")
  resp=$("${curl[@]}")
  if [[ $resp == "[]" ]]; then
    return 1
  else
    return 0
  fi
}

DeleteHost() {
  curl=("curl" -skSu "$i2Auth" -H 'Accept: application/json' -X DELETE "$1")
  resp=$("${curl[@]}")
  if [[ $resp == "[]" ]]; then
    return 1
  else
    return 0
  fi
}

UpdateObject() {
  curl=("curl" -skSu "$i2Auth" -H 'Accept: application/json' -X POST "$@")
  resp=$("${curl[@]}")
  if [[ $resp == "[]" ]]; then
    return 1
  else
    return 0
  fi
}

object="host-$RANDOM"

while true; do

  printf "\nCreating Icinga 2 '%s' config file.\n" $cfdPath/$object".conf"
  cat >$cfdPath/$object".conf" <<-EOF
    object Host "$object" {
      display_name = "icingaservice"
      check_command = "dummy"
      enable_active_checks = false
      enable_notifications = "1.000000"
    }
EOF

  if UpdateObject "https://$i2Host/v1/actions/restart-process"; then
    printf "\nIcinga 2 daemon successfully restarted.\n"
  else
    printf "\nFailed to restart Icinga 2 daemon.\n"
  fi

  printf "Sleep until Icinga 2 has been successfully restarted.\n"
  sleep 30
  printf "Removing Icinga 2 '%s' config file.\n" $cfdPath/$object".conf"
  rm $cfdPath/$object".conf"

  if UpdateObject "https://$i2Host/v1/actions/restart-process"; then
    printf "\nIcinga 2 daemon successfully restarted.\n"
  else
    printf "\nFailed to restart Icinga 2 daemon.\n"
  fi

  printf "Sleep until Icinga 2 has been successfully restarted.\n"
  sleep 10

  # Querying Icinga Web 2 for host created using config file
  if GetHost "$iw2Auth" "http://$iw2Host/icingaweb2/monitoring/list/hosts?host=$object&modifyFilter=1"; then
    printf "\nHost %s object can be found in Icinga Web 2.\n" $object
  else
    printf "\nHost %s object doesn't exists in Icinga Web 2.\n" $object
  fi

  # Create host via the API
  if CreateHost "https://$i2Host/v1/objects/hosts/$object" -d '{ "attrs": {
    "check_command": "hostalive",
    "address": "10.211.55.12",
    "display_name": "icingaservice",
    "vars": { "os": "Linux"} } }'; then
    printf "\nHost '%s' object successfully created via API.\n" $object
  else
    printf "\nFailed to create '%s' object of type Host.\n" $object
  fi

  printf "Sleep until Icinga 2 reloaded all necessary jobs.\n"
  sleep 10

  # Querying Icinga Web 2 for host created via the API
  if GetHost "$iw2Auth" "http://$iw2Host/icingaweb2/monitoring/list/hosts?host=$object&modifyFilter=1"; then
    printf "\nHost %s object can be found in Icinga Web 2.\n" $object
  else
    printf "\nHost %s object doesn't exists in Icinga Web 2.\n" $object
  fi

  # Querying Icinga2 for hos object host $object
  if GetHost "$i2Auth" "https://$i2Host/v1/objects/hosts/$object"; then
    printf "\nHost %s object can be found in Icinga 2.\n" $object
  else
    printf "\nHost %s object doesn't exists.\n" $object
  fi

  # Create a new host with new object name via the API
  if CreateHost "https://$i2Host/v1/objects/hosts/icinga2" -d '{ "attrs": {
      "check_command": "hostalive",
      "address": "10.211.55.10",
      "display_name": "newicingaservice",
      "vars": { "os": "MacOs"} }
    }'; then
    printf "\nHost 'icinga2' object successfully created via API.\n"
  else
    printf "\nFailed to create 'icinga2' object of type Host.\n"
  fi

  printf "Sleep until Host 'icinga2' object is visible in Icinga Web 2.\n"
  sleep 10

  # Querying Icinga Web 2 for host object 'icinga2'
  if GetHost "$iw2Auth" "http://$iw2Host/icingaweb2/monitoring/list/hosts?host=icinga2&modifyFilter=1"; then
    printf "\nIcinga Web 2 host 'icinga2' object can be found.\n"
  else
    printf "\nIcinga Web 2 host 'icinga2' object doesn't exists.\n"
  fi

  # Querying Icinga2 for hos object 'icinga2'
  if GetHost "$i2Auth" "https://$i2Host/v1/objects/hosts/icinga2"; then
    printf "\nIcinga 2 API host 'icinga2' object found.\n"
  else
    printf "\nIcinga 2 API host 'icinga2 object doesn't exists.\n"
  fi

  # Schedule a downtime
  if UpdateObject "https://$i2Host/v1/actions/schedule-downtime" -d '{
    "type": "Host",
    "start_time": '$(date +%s)',
    "end_time": '$(date -v +1H +%s)',
    "author":"icingaadmin",
    "comment":"42"
  }'; then
    printf "\nDowntime of type 'Host' successfully scheduled.\n"
  else
    printf "\nFailed to schedule a downtime of type 'Host'.\n"
  fi

  # Send custom notification
  if UpdateObject "https://$i2Host/v1/actions/send-custom-notification" -d '{
      "type": "Host", "author": "icingaadmin",
      "comment": "System is going down for maintenance",
      "force": true,
  }'; then
    printf "\nSending 'custom' Notification of type 'Host'.\n"
  else
    printf "\nFailed to send 'custom' Notification of type 'Host'.\n"
  fi

  printf "\nSleep until all created hosts are visible in Icinga Web 2.\n"
  sleep 30

  if DeleteHost "https://$i2Host/v1/objects/hosts/$object?cascade=1"; then
    printf "\nHost '%s' object successfully deleted.\n" $object
  else
    printf "\nFailed to delete Host '%s' object.\n" $object
  fi

  if DeleteHost "https://$i2Host/v1/objects/hosts/icinga2?cascade=1"; then
    printf "Host 'icinga2' object successfully deleted.\n"
  else
    printf "Failed to delete Host 'icinga2' object.\n"
  fi

  printf "\nSleep until all deleted Hosts are also deleted from IDO.\n"
  sleep 30
done
