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


inputs = [[1], [2], [10], [35], [37], [7919], [10007], [524287], [7920], [True], [10006], [524288], [7921], [10008], [10005], [7918], [524289], [10004], [524286], [524290], [7922], [7923], [10009], [7917], [524285], [7916], [10003], [524284], [7924], [10010], [7915], [524283], [7925], [10011], [10002], [7914], [7926], [524291], [10012], [10001], [524292], [10000], [7927], [7928], [9999], [7929], [524293], [7913], [10013], [10014], [524282], [7912], [63], [9998], [62], [524281], [23], [64], [58], [60], [16], [59], [9997], [57], [10015], [61], [20], [56], [21], [7930], [55], [7911], [54], [19], [53], [9996], [524280], [22], [9995], [96], [9994], [7931], [10016], [524279], [97], [9993], [15], [94], [65], [93], [29], [66], [30], [92], [41], [95], [91], [14], [6], [524294], [4], [524278], [524277], [90], [524275], [5], [524295], [3]]
results = [True, False, True, True, False, False, False, False, True, True, True, True, True, True, True, True, True, True, True, True, True, True, False, True, True, True, True, True, True, True, True, True, True, True, True, True, True, True, True, True, True, True, False, True, True, True, True, True, True, True, True, True, True, True, True, True, False, True, True, True, True, False, True, True, True, False, True, True, True, True, True, True, True, False, False, True, True, True, True, True, True, True, True, True, False, True, True, True, True, True, False, True, True, True, False, True, True, True, True, True, True, True, True, True, True, False, True, False]
for i, (inp, exp) in enumerate(zip(inputs, results)):
    assertion(is_not_prime(*inp), exp, 0)
