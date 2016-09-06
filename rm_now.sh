#!/bin/bash

ed $1 <<XXxx
g/__now__/d
w
q
XXxx
