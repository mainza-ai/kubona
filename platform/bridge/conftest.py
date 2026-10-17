"""Bridge standalone tests import sibling modules top-level
(``from chemistry import ...``). This keeps that working no matter which
directory pytest is invoked from — without it, collection depends on the
caller's PYTHONPATH (tribal knowledge that breaks CI the day someone adds
the bridge to testpaths)."""

import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
