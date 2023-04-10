import yaml
import sys



def main():
    if len(sys.argv)!=3:
        print("Error: Please enter the correct f and N!")
        return
    try:
        f=int(sys.argv[1])
        n=int(sys.argv[2])
    except ValueError:
        print("Error: f and N should be integers!")
        return

    leader_schedule=[]
    cluster=[]
    for i in range(1, n+1):
        leader_schedule.append(i)
        port=str(8080+i-1)
        cluster.append({
            "id": i,
            "listen-address": "localhost:"+port,
            "privatekeypath": "/home/lmh/Desktop/consensus_code/keys-go-hotstuff/keys/r"+str(i)+".key"
        })

    data = {
        "hotstuff":{
            "type": "basic",
            "batch-size": 10,
            "batchtimeout": "1s",
            "timeout": "2s",
            "pubkeypath": "/home/lmh/Desktop/consensus_code/keys-go-hotstuff/keys/pub.key",
            "N": n,
            "f": f,
            "leader-schedule": leader_schedule,
            "cluster": cluster
        }
    }
    

    with open('hostuff.yaml', mode="w+", encoding='utf-8') as file:
        yaml.dump(data=data, stream=file, allow_unicode=True)

if __name__=='__main__':
    main()