#!/bin/bash

tcld gen ca --org temporal -d 1y --ca-cert ca.pem --ca-key ca.key
tcld gen leaf --org temporal -d 364d --ca-cert ca.pem --ca-key ca.key --cert client.pem --key client.key
