import os
from typing import Iterable, List


def build_data_yaml(names: Iterable[str]) -> str:
    ordered = list(names)
    return "train: images/train\nval: images/val\nnames:\n  - " + "\n  - ".join(ordered) + "\n"


def build_package_tree(workdir: str, names: List[str]) -> None:
    os.makedirs(os.path.join(workdir, "images"), exist_ok=True)
    os.makedirs(os.path.join(workdir, "labels"), exist_ok=True)
    with open(os.path.join(workdir, "data.yaml"), "w", encoding="utf-8") as f:
        f.write(build_data_yaml(names))
