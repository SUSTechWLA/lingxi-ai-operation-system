package com.lingxi.ai.orchestrator.repository;

import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;
import org.springframework.stereotype.Repository;

import java.util.List;

@Repository
public interface NodeRepository extends JpaRepository<Node, String> {

    List<Node> findByTaskId(String taskId);

    List<Node> findByStatus(NodeStatus status);

    List<Node> findByTaskIdAndStatus(String taskId, NodeStatus status);

    List<Node> findByTaskIdAndStatusIn(String taskId, List<NodeStatus> statuses);

    @Query("SELECT n FROM Node n WHERE n.status = 'READY'")
    List<Node> findReadyNodes();

    @Query("SELECT n FROM Node n WHERE n.status = 'CREATED'")
    List<Node> findCreatedNodes();

    @Query("SELECT n FROM Node n WHERE n.id IN (" +
            "SELECT d.childNodeId FROM NodeDependency d WHERE d.parentNodeId = :parentNodeId)")
    List<Node> findChildNodes(@Param("parentNodeId") String parentNodeId);

    @Query("SELECT d FROM NodeDependency d WHERE d.childNodeId = :childNodeId")
    List<com.lingxi.ai.orchestrator.entity.NodeDependency> findByChildNodeId(@Param("childNodeId") String childNodeId);

    @Modifying
    @Query("UPDATE Node n SET n.status = :newStatus, n.version = n.version + 1 " +
            "WHERE n.id = :nodeId AND n.version = :version")
    int updateStatusWithLock(@Param("nodeId") String nodeId,
                              @Param("newStatus") NodeStatus newStatus,
                              @Param("version") Integer version);
}
