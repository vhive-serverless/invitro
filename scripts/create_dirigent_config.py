import os
import pandas as pd

path = '/home/lcvetkovic/Downloads/day6_hour8_samples/samples'

for files in os.listdir(path):
    df = pd.read_csv(f'{path}/{files}/invocations.csv')
    dirigent = pd.DataFrame()

    for index, row in df.iterrows():
        function = row['HashFunction']

        dirigent = dirigent.append( {
            'HashFunction': function,
            'Image': 'docker.io/cvetkovic/dirigent_trace_function:latest',
            'Port': 80,
            'Protocol': 'tcp',
            'ScalingUpperBound': 10000,
            'ScalingLowerBound': 0,
            'IterationMultiplier': 155,
        }, ignore_index=True)

    dirigent.to_csv(f'{path}/{files}/dirigent.csv', index=False)