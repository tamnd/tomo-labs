import numpy as np
from math import inf

def is_floats(x) -> bool:
    # check if it is float; List[float]; Tuple[float]
    if isinstance(x, float):
        return True
    if isinstance(x, (list, tuple)):
        return all(isinstance(i, float) for i in x)
    if isinstance(x, np.ndarray):
        return x.dtype == np.float64 or x.dtype == np.float32
    return False


def assertion(out, exp, atol):
    if atol == 0 and is_floats(exp):
        atol = 1e-6
    if out != exp and atol != 0:
        assert np.allclose(out, exp, rtol=1e-07, atol=atol)
    else:
        assert out == exp, f"out: {out}, exp: {exp}"


inputs = [[13, 9], [15, 8], [2, 4], [2, 3], [5, 1], [1, 5], [0, 0], [-10, 10], [100, 100], [-50, -100], [123456789, -987654321], [-123456789, -987654321], [1000000000, 1000000001], [0, 1], [-100, -100], [-123456789, 0], [-10, -987654321], [1000000000, 100], [10, 0], [-101, -100], [1000000000, 1000000000], [10, 10], [-1, 0], [-101, 100], [-2, -2], [-123456789, -10], [-50, -50], [-50, -101], [-101, 1000000001], [1, -987654320], [-101, -101], [-11, -987654321], [-50, -102], [-3, 1], [-987654321, -987654320], [-987654321, -100], [0, 1000000001], [-50, -987654321], [-102, -987654321], [-102, 1], [1000000000, 10], [123456789, 1], [-10, -10], [10, -987654320], [-1, 1], [-101, -102], [-11, 0], [-1, -100], [-3, -987654320], [1, -50], [-123456789, -50], [-2, -1], [123456789, -2], [-2, -4], [-101, 10], [-2, 123456789], [-2, -987654321], [-1, -1], [1000000001, 1000000001], [-1, -2], [-50, 1000000000], [-3, -987654321], [-51, -50], [1, -1], [-100, -2], [1000000000, 101], [1000000000, -51], [-49, -102], [-102, -100], [-123456789, -123456789], [-51, -51], [-9, 10], [-4, -101], [-102, -101], [2, 2], [-50, -99], [-1, 101], [-2, -11], [-3, -2], [-987654321, -10], [-100, -49], [False, True], [True, True], [-987654321, -987654321], [123456789, -4], [123456789, 100], [9, 10], [-987654321, 1], [-3, -1], [-102, -102], [101, -101], [11, 10], [-50, -49], [False, False], [123456789, -50], [-10, 1], [-3, -51], [1, -10], [-10, 11], [-102, 2], [8, 8], [123456787, 1], [-987654321, 101], [9, -123456788], [8, -50], [-101, -3], [-123456788, 1000000000], [-12, 0], [-50, -1], [-987654320, 2], [-4, -123456789], [-2, -10], [-12, -101], [-9, -1]]
results = [True, False, False, True, True, True, False, False, False, False, False, False, True, True, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, True, False, False, False, False, False, True, False, True, False, False, False, False, False, True, False, False, False, False, False, False, False, False, False, False, False, False, False, True, False, False, False, False, False, False, False, True, False, False, False, False, False, False, True, False, False, True, True, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, False, True, False, True]
for i, (inp, exp) in enumerate(zip(inputs, results)):
    assertion(differ_At_One_Bit_Pos(*inp), exp, 0)
