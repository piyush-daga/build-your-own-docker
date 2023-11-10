# Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...

from os import commandLineParams
from osproc import execProcess


let command = commandLineParams()[2]
let args = commandLineParams()[3..^1]

let output = execProcess(command, "", args, options={})
echo output
