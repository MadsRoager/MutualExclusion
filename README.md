# MutualExclusion

To execute MutualExclusion:

- Open the number of terminals that you want to use for the program
- In each terminal navigate to the folder MutualExclusion, and provide the ports that the program should use as arguments 
when running the program. The first port should be the port that the terminal itself should use, and the following should be the ones that it 
should try to dial. 
E.g. if you want to use port 8000, 8001 and 8002 type the following in the 3 different terminals.

  - Terminal 1: go run . 8000 8001 8002

  - Terminal 2: go run . 8001 8002 8000

  - Terminal 3: go run . 8002 8000 8001
