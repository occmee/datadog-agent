# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019 Datadog, Inc.
from __future__ import print_function
import _util

if __name__ == "__main__":
    _util.print_foo()
    print("Constant from _util %d" % _util.constant_number)
